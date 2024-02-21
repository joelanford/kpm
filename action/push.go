package action

import (
	"context"
	"io"
	"os"

	"github.com/joelanford/kpm/internal/tar"
	kpmoci "github.com/joelanford/kpm/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"
)

type PushFunc func(context.Context, kpmoci.Artifact) (string, ocispec.Descriptor, error)

func Write(w io.Writer) PushFunc {
	return func(ctx context.Context, a kpmoci.Artifact) (string, ocispec.Descriptor, error) {
		tmpDir, err := os.MkdirTemp("", "kpm-")
		if err != nil {
			return "", ocispec.Descriptor{}, err
		}
		defer os.RemoveAll(tmpDir)

		tmpStore, err := oci.NewWithContext(ctx, tmpDir)
		if err != nil {
			return "", ocispec.Descriptor{}, err
		}

		tag, desc, err := Push(tmpStore, kpmoci.PushOptions{})(ctx, a)
		if err != nil {
			return "", ocispec.Descriptor{}, err
		}
		if err := tar.Directory(w, os.DirFS(tmpDir)); err != nil {
			return "", ocispec.Descriptor{}, err
		}
		return tag, desc, nil
	}
}

func Push(t oras.Target, pushOpts kpmoci.PushOptions) PushFunc {
	return func(ctx context.Context, a kpmoci.Artifact) (string, ocispec.Descriptor, error) {
		desc, err := kpmoci.Push(ctx, a, t, pushOpts)
		if err != nil {
			return "", ocispec.Descriptor{}, err
		}
		tag := a.Tag()
		if err := t.Tag(ctx, desc, tag); err != nil {
			return "", ocispec.Descriptor{}, err
		}

		return tag, desc, nil
	}
}
