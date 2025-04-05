package kpm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"

	"github.com/containers/image/v5/docker/reference"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/mutate"
	"github.com/google/go-containerregistry/pkg/v1/partial"
	"github.com/google/go-containerregistry/pkg/v1/types"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"

	"github.com/joelanford/kpm/internal/ociutil"
	"github.com/joelanford/kpm/internal/remote"
	"github.com/joelanford/kpm/internal/tar"
)

type KPM struct {
	Descriptor  ocispec.Descriptor
	Reference   reference.NamedTagged
	Annotations map[string]string
	store       *oci.ReadOnlyStore
	storePath   string
	manifest    ocispec.Manifest
}

func Open(ctx context.Context, kpmFilePath string) (*KPM, error) {
	kpmStore, err := oci.NewFromTar(ctx, kpmFilePath)
	if err != nil {
		return nil, fmt.Errorf("failed to open kpm file: %w", err)
	}

	var tag string
	if err := kpmStore.Tags(ctx, "", func(tags []string) error {
		if len(tags) != 1 {
			return fmt.Errorf("expected exactly one tag, got %d", len(tags))
		}
		tag = tags[0]
		return nil
	}); err != nil {
		return nil, fmt.Errorf("failed to read tags: %w", err)
	}

	tagRef, err := ociutil.ParseNamedTagged(tag)
	if err != nil {
		return nil, err
	}

	desc, err := kpmStore.Resolve(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("kpm artifact not found: %w", err)
	}

	manifestReader, err := kpmStore.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest %q: %w", desc.Digest, err)
	}
	manifestData, err := content.ReadAll(manifestReader, desc)
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return nil, fmt.Errorf("failed to decode manifest: %w", err)
	}

	return &KPM{
		Descriptor:  desc,
		Reference:   tagRef,
		Annotations: manifest.Annotations,
		store:       kpmStore,
		storePath:   kpmFilePath,
		manifest:    manifest,
	}, nil
}

func (k *KPM) Push(ctx context.Context, opts oras.CopyGraphOptions) error {
	dest, err := remote.NewRepository(k.Reference.Name())
	if err != nil {
		return fmt.Errorf("failed to setup client for destination repository: %w", err)
	}
	if err := oras.CopyGraph(ctx, k.store, dest, k.Descriptor, opts); err != nil {
		return fmt.Errorf("failed to copy kpm to destination: %w", err)
	}
	if err := dest.Tag(ctx, k.Descriptor, k.Reference.Tag()); err != nil {
		return fmt.Errorf("failed to tag copied kpm artifact: %w", err)
	}
	return nil
}

func (k *KPM) Mount(outputDir string) (string, error) {
	img, err := partial.CompressedToImage(ggcrImage{k})
	if err != nil {
		return "", fmt.Errorf("failed to convert kpm to image: %w", err)
	}

	if outputDir == "" {
		tmpDir, err := os.MkdirTemp("", "kpm-mount-")
		if err != nil {
			return "", fmt.Errorf("failed to create temporary directory: %w", err)
		}
		outputDir = tmpDir
	} else {
		if _, err := os.Stat(outputDir); err == nil {
			return "", fmt.Errorf("%q already exists", outputDir)
		}
		if err := os.MkdirAll(outputDir, 0755); err != nil {
			return "", fmt.Errorf("failed to create output directory: %w", err)
		}
	}

	if err := tar.Extract(mutate.Extract(img), outputDir); err != nil {
		os.RemoveAll(outputDir)
		return "", fmt.Errorf("failed to extract image: %w", err)
	}
	return outputDir, nil
}

var (
	_ partial.CompressedImageCore = (*ggcrImage)(nil)
	_ partial.CompressedLayer     = (*ggcrLayer)(nil)
)

type ggcrImage struct {
	*KPM
}

func (g ggcrImage) RawConfigFile() ([]byte, error) {
	rc, err := g.store.Fetch(context.Background(), g.manifest.Config)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config: %w", err)
	}
	defer rc.Close()
	return content.ReadAll(rc, g.manifest.Config)
}

func (g ggcrImage) MediaType() (types.MediaType, error) {
	return types.MediaType(g.manifest.Config.MediaType), nil
}

func (g ggcrImage) RawManifest() ([]byte, error) {
	rc, err := g.store.Fetch(context.Background(), g.Descriptor)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch manifest: %w", err)
	}
	defer rc.Close()
	return content.ReadAll(rc, g.Descriptor)
}

func (g ggcrImage) LayerByDigest(hash v1.Hash) (partial.CompressedLayer, error) {
	for _, layerDesc := range g.manifest.Layers {
		if layerDesc.Digest.String() == hash.String() {
			return ggcrLayer{store: g.store, desc: layerDesc}, nil
		}
	}
	return nil, fmt.Errorf("layer with digest %q not found", hash)
}

type ggcrLayer struct {
	store *oci.ReadOnlyStore
	desc  ocispec.Descriptor
}

func (g ggcrLayer) Digest() (v1.Hash, error) {
	return v1.NewHash(g.desc.Digest.String())
}

func (g ggcrLayer) Compressed() (io.ReadCloser, error) {
	if g.desc.MediaType != string(types.OCILayer) && g.desc.MediaType != string(types.DockerLayer) {
		return nil, fmt.Errorf("unsupported media type %q", g.desc.MediaType)
	}
	return g.store.Fetch(context.Background(), g.desc)
}

func (g ggcrLayer) Size() (int64, error) {
	return g.desc.Size, nil
}

func (g ggcrLayer) MediaType() (types.MediaType, error) {
	return types.MediaType(g.desc.MediaType), nil
}
