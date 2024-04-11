package action

import (
	"context"
	"io"
	"io/fs"

	buildv1 "github.com/joelanford/kpm/build/v1"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type BuildBundle struct {
	SpecFileReader io.Reader
	BundleFS       fs.FS
	PushFunc       PushFunc
}

func (a *BuildBundle) Run(ctx context.Context) (string, ocispec.Descriptor, error) {
	bundle, err := buildv1.Bundle(a.SpecFileReader, a.BundleFS)
	if err != nil {
		return "", ocispec.Descriptor{}, err
	}

	return a.PushFunc(ctx, bundle)
}
