package action

import (
	"context"
	"io"
	"io/fs"

	buildv1 "github.com/joelanford/kpm/build/v1"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type BuildCatalog struct {
	SpecFileReader io.Reader
	CatalogFS      fs.FS
	PushFunc       PushFunc
}

func (a *BuildCatalog) Run(ctx context.Context) (string, ocispec.Descriptor, error) {
	bundle, err := buildv1.Catalog(a.SpecFileReader, a.CatalogFS)
	if err != nil {
		return "", ocispec.Descriptor{}, err
	}

	return a.PushFunc(ctx, bundle)
}
