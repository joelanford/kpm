package registryv1

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"io"
	"io/fs"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"

	"github.com/joelanford/kpm/internal/util/tar"
)

type RegistryV1Writer struct {
	author      string
	labels      map[string]string
	annotations map[string]string
	root        fs.FS
}

func (b *RegistryV1Writer) ArtifactType() string {
	return ""
}
func (b *RegistryV1Writer) Annotations() map[string]string {
	return b.annotations
}
func (b *RegistryV1Writer) Subject() *ocispec.Descriptor {
	return nil
}

func (b *RegistryV1Writer) PushConfigAndLayers(ctx context.Context, pusher content.Pusher) (ocispec.Descriptor, []ocispec.Descriptor, error) {
	var layerData bytes.Buffer
	diffIDHash := sha256.New()

	if err := func() error {
		gzipWriter := gzip.NewWriter(&layerData)
		defer gzipWriter.Close()
		mw := io.MultiWriter(diffIDHash, gzipWriter)
		return tar.Directory(mw, b.root)
	}(); err != nil {
		return ocispec.Descriptor{}, nil, err
	}

	cfg := ocispec.Image{
		Author: b.author,
		Config: ocispec.ImageConfig{
			Labels: b.labels,
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
