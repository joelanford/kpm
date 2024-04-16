package v1

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"os"

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

type buildOptions struct {
	SpecReader io.Reader
	Log        func(string, ...interface{})
}

func getConfigData(annotations map[string]string, blobData []byte) ([]byte, error) {
	config := ocispec.Image{
		Config: ocispec.ImageConfig{
			Labels: annotations,
		},
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: []digest.Digest{digest.FromBytes(blobData)},
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
	// TODO: figure out why gzipping is causing a problem with diffIDs not matching
	//       btw, this somehow works with the blob being tar, but the mediatype being tar.gz
	//gzw := gzip.NewWriter(buf)
	tw := tar.NewWriter(buf)
	if err := kpmtar.AddFS(tw, os.DirFS(tmpDir)); err != nil {
		return nil, err
	}

	if err := tw.Close(); err != nil {
		return nil, err
	}
	//if err := gzw.Close(); err != nil {
	//	return nil, err
	//}
	return buf.Bytes(), nil
}
