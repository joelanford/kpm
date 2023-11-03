package oci

import (
	"context"
	"io"

	"github.com/docker/docker/pkg/progress"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
)

func newProgressStore(base content.ReadOnlyStorage, out progress.Output) content.ReadOnlyStorage {
	return &progressStore{
		base: base,
		out:  out,
	}
}

type progressStore struct {
	base content.ReadOnlyStorage
	out  progress.Output
}

func (s *progressStore) Exists(ctx context.Context, desc ocispec.Descriptor) (bool, error) {
	return s.base.Exists(ctx, desc)
}

func (s *progressStore) Fetch(ctx context.Context, desc ocispec.Descriptor) (io.ReadCloser, error) {
	rc, err := s.base.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}

	return progress.NewProgressReader(rc, s.out, desc.Size, idForDesc(desc), "Pushing "), nil
}

func idForDesc(desc ocispec.Descriptor) string {
	return desc.Digest.String()[7:19]
}
