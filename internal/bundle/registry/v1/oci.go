package v1

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"

	"github.com/joelanford/kpm/internal/pkg/util/tar"
)

func (b *Bundle) name() string {
	return b.metadata.annotationsFile.Annotations[AnnotationPackage]
}

func (b *Bundle) tag() string {
	return b.csv.Spec.Version.String()
}

func (b *Bundle) ID() string {
	return fmt.Sprintf("%s.v%s", b.name(), b.tag())
}

func (b *Bundle) imageNameTag() string {
	return fmt.Sprintf("%s:%s", b.name(), b.tag())
}

func (b *Bundle) MarshalOCI(ctx context.Context, target oras.Target) (ocispec.Descriptor, error) {
	config, layers, err := b.pushConfigAndLayers(ctx, target)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	manifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    config,
		Layers:    layers,
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	desc, err := oras.PushBytes(ctx, target, ocispec.MediaTypeImageManifest, manifestData)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push bundle: %v", err)
	}
	if err := target.Tag(ctx, desc, b.imageNameTag()); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to tag bundle: %v", err)
	}
	return desc, nil
}

func (b *Bundle) pushConfigAndLayers(ctx context.Context, pusher content.Pusher) (ocispec.Descriptor, []ocispec.Descriptor, error) {
	var layerData bytes.Buffer
	diffIDHash := sha256.New()

	if err := func() error {
		gzipWriter := gzip.NewWriter(&layerData)
		defer gzipWriter.Close()
		mw := io.MultiWriter(diffIDHash, gzipWriter)
		return tar.Directory(mw, b.fsys)
	}(); err != nil {
		return ocispec.Descriptor{}, nil, err
	}

	cfg := ocispec.Image{
		Config: ocispec.ImageConfig{
			Labels: b.metadata.annotationsFile.Annotations,
		},
		RootFS: ocispec.RootFS{
			Type: "layers",
			DiffIDs: []digest.Digest{
				digest.NewDigest(digest.SHA256, diffIDHash),
			},
		},
	}
	cfgData, err := json.Marshal(cfg)
	if err != nil {
		return ocispec.Descriptor{}, nil, err
	}
	cfgDesc, err := oras.PushBytes(ctx, pusher, ocispec.MediaTypeImageConfig, cfgData)
	if err != nil {
		return ocispec.Descriptor{}, nil, err
	}
	layerDesc, err := oras.PushBytes(ctx, pusher, ocispec.MediaTypeImageLayerGzip, layerData.Bytes())
	if err != nil {
		return ocispec.Descriptor{}, nil, err
	}
	return cfgDesc, []ocispec.Descriptor{layerDesc}, nil
}
