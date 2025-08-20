package v1

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"testing/fstest"
	"time"

	"github.com/containerd/containerd/archive"
	"github.com/containerd/containerd/archive/compression"
	"github.com/go-logr/logr"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"

	tarutil "github.com/operator-framework/kpm/internal/pkg/util/tar"
)

const (
	mediaType          = "registry+v1"
	manifestsDirectory = "manifests/"
	metadataDirectory  = "metadata/"
)

type Bundle struct {
	manifests *manifests
	metadata  *metadata
}

type BundleLoader interface {
	Load(context.Context) (*Bundle, error)
}

type bundleFSLoader struct {
	fsys fs.FS
}

func NewBundleFSLoader(fsys fs.FS) BundleLoader {
	return &bundleFSLoader{fsys: fsys}
}

func (b *bundleFSLoader) Load(_ context.Context) (*Bundle, error) {
	manifestsFS, manifestsFSErr := fs.Sub(b.fsys, filepath.Clean(manifestsDirectory))
	metadataFS, metadataFSErr := fs.Sub(b.fsys, filepath.Clean(metadataDirectory))
	if err := errors.Join(manifestsFSErr, metadataFSErr); err != nil {
		return nil, err
	}

	manifestsLoader := &manifestsFSLoader{manifestsFS}
	metadataLoader := &metadataFSLoader{metadataFS}

	bundleManifests, manifestsErr := manifestsLoader.Load()
	bundleMetadata, metadataErr := metadataLoader.Load()
	if err := errors.Join(manifestsErr, metadataErr); err != nil {
		return nil, err
	}

	return &Bundle{manifests: bundleManifests, metadata: bundleMetadata}, nil
}

type bundleOCILoader struct {
	src oras.ReadOnlyTarget
	ref string
}

func NewBundleOCILoader(src oras.ReadOnlyTarget, ref string) BundleLoader {
	return &bundleOCILoader{src: src, ref: ref}
}

func (b *Bundle) tag() string {
	return b.manifests.CSV().Value().Spec.Version.String()
}

func (b *Bundle) ID() string {
	return fmt.Sprintf("%s.v%s", b.metadata.PackageName(), b.tag())
}

func (b *Bundle) imageNameTag() string {
	return fmt.Sprintf("%s:%s", b.metadata.PackageName(), b.tag())
}

func (b *bundleOCILoader) Load(ctx context.Context) (*Bundle, error) {
	// Resolve the reference
	desc, err := b.src.Resolve(ctx, b.ref)
	if err != nil {
		return nil, fmt.Errorf("error resolving ref %q: %v", b.ref, err)
	}

	// A registry+v1 bundle is expected to be an OCI manifest (or compatible), not an OCI Index, not a Docker Manifest List.
	if desc.MediaType != ocispec.MediaTypeImageManifest {
		return nil, fmt.Errorf(`unexpected media type %q: registry+v1 bundles are expected to be OCI image manifests`, desc.MediaType)
	}

	// Unmarshal the manifest
	manifestRC, err := b.src.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("error fetching manifest: %v", err)
	}
	defer manifestRC.Close()
	manifestBytes, err := io.ReadAll(manifestRC)
	if err != nil {
		return nil, fmt.Errorf("error reading manifest: %v", err)
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return nil, fmt.Errorf("error unmarshaling manifest: %v", err)
	}

	// Unmarshal the image config
	configRC, err := b.src.Fetch(ctx, manifest.Config)
	if err != nil {
		return nil, fmt.Errorf("error fetching config: %v", err)
	}
	defer configRC.Close()
	configBytes, err := io.ReadAll(configRC)
	if err != nil {
		return nil, fmt.Errorf("error reading config: %v", err)
	}
	var config ocispec.Image
	if err := json.Unmarshal(configBytes, &config); err != nil {
		return nil, fmt.Errorf("error unmarshaling config: %v", err)
	}

	// Un-tar the layers
	tmpDir, err := os.MkdirTemp("", "kpm-unmarshal-oci")
	if err != nil {
		return nil, fmt.Errorf("error creating temp directory: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			logr.FromContextOrDiscard(ctx).Error(err, "failed to remove temporary directory", "tmpDir", tmpDir)
		}
	}()
	for _, layer := range manifest.Layers {
		layerRC, err := b.src.Fetch(ctx, layer)
		if err != nil {
			return nil, fmt.Errorf("error fetching layer: %v", err)
		}
		defer layerRC.Close()

		decompressed, err := compression.DecompressStream(layerRC)
		if err != nil {
			return nil, fmt.Errorf("error decompressing layer: %v", err)
		}
		defer decompressed.Close()
		if _, err := archive.Apply(ctx, tmpDir, decompressed, archive.WithFilter(func(header *tar.Header) (bool, error) {
			header.PAXRecords = nil
			header.Xattrs = nil
			header.ChangeTime = time.Time{}
			header.ModTime = time.Time{}
			header.ChangeTime = time.Time{}
			header.Uid = os.Getuid()
			header.Gid = os.Getgid()
			header.Mode |= 0600
			return true, nil
		})); err != nil {
			return nil, fmt.Errorf("error applying tar layer: %v", err)
		}
	}
	return NewBundleFSLoader(os.DirFS(tmpDir)).Load(ctx)
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

func (b *Bundle) toFS() fs.FS {
	fsys := fstest.MapFS{}
	b.manifests.addToFS(fsys)
	b.metadata.addToFS(fsys)
	return fsys
}

func (b *Bundle) pushConfigAndLayers(ctx context.Context, pusher content.Pusher) (ocispec.Descriptor, []ocispec.Descriptor, error) {
	var layerData bytes.Buffer
	diffIDHash := sha256.New()

	bundleFS := b.toFS()
	if err := func() error {
		gzipWriter := gzip.NewWriter(&layerData)
		defer gzipWriter.Close()
		mw := io.MultiWriter(diffIDHash, gzipWriter)
		return tarutil.Directory(mw, bundleFS)
	}(); err != nil {
		return ocispec.Descriptor{}, nil, err
	}

	annotationsFile := b.metadata.Annotations()
	cfg := ocispec.Image{
		Config: ocispec.ImageConfig{
			Labels: annotationsFile.Value().Annotations,
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
