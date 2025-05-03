package v1

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"

	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"

	bundlev1alpha1 "github.com/joelanford/kpm/internal/api/bundle/v1alpha1"
	"github.com/joelanford/kpm/internal/pkg/util/tar"
)

func (b *Bundle) ID() bundlev1alpha1.ID {
	return b.id
}

func (b *Bundle) MarshalOCI(ctx context.Context, pusher content.Pusher) (ocispec.Descriptor, error) {
	config, layers, err := b.pushConfigAndLayers(ctx, pusher)
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
	return oras.PushBytes(ctx, pusher, ocispec.MediaTypeImageManifest, manifestData)
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
			Labels: b.metadata.annotations.Annotations,
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
