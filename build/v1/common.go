package v1

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"

	"github.com/containers/image/v5/docker/reference"
	"github.com/joelanford/kpm/internal/fsutil"
	kpmtar "github.com/joelanford/kpm/internal/tar"
	"github.com/joelanford/kpm/oci"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type ArtifactBuilder interface {
	BuildArtifact(ctx context.Context) (oci.Artifact, error)
}

type BuildOption func(*buildOptions)

func WithSpecReader(r io.Reader) BuildOption {
	return func(opts *buildOptions) {
		opts.SpecReader = r
	}
}

func WithLog(log func(string, ...interface{})) BuildOption {
	return func(opts *buildOptions) {
		opts.Log = log
	}
}

func WithOriginRepository(ref reference.Named) BuildOption {
	return func(opts *buildOptions) {
		opts.OriginRepository = ref
	}
}

type buildOptions struct {
	SpecReader       io.Reader
	OriginRepository reference.Named
	Log              func(string, ...interface{})
}

func getConfigData(annotations map[string]string, blobData []byte) ([]byte, error) {
	gzr, err := gzip.NewReader(bytes.NewReader(blobData))
	if err != nil {
		return nil, err
	}
	blobDiffID, err := digest.FromReader(gzr)
	if err != nil {
		return nil, err
	}
	config := ocispec.Image{
		Config: ocispec.ImageConfig{
			Labels: annotations,
		},
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: []digest.Digest{blobDiffID},
		},
		History: []ocispec.History{{
			CreatedBy: "kpm",
		}},
		Platform: ocispec.Platform{
			OS: "linux",
		},
	}
	return json.Marshal(config)
}

func getBlobData(fsys ...fs.FS) ([]byte, error) {
	tmpDir, err := os.MkdirTemp("", "kpm-blob-data-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	for _, f := range fsys {
		if err := fsutil.Write(tmpDir, f); err != nil {
			return nil, err
		}
	}

	buf := &bytes.Buffer{}
	gzw := gzip.NewWriter(buf)
	tw := tar.NewWriter(gzw)
	if err := kpmtar.AddFS(tw, os.DirFS(tmpDir)); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gzw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
