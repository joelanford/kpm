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

	Log func(string, ...interface{})
}

func (a *BuildBundle) Run(ctx context.Context) (string, ocispec.Descriptor, error) {
	opts := []buildv1.BuildOption{}
	if a.SpecFileReader != nil {
		opts = append(opts, buildv1.WithSpecReader(a.SpecFileReader))
	}
	if a.Log != nil {
		opts = append(opts, buildv1.WithLog(a.Log))
	}

	bundle, err := buildv1.NewBundleBuilder(a.BundleFS, opts...).BuildArtifact(ctx)
	if err != nil {
		return "", ocispec.Descriptor{}, err
	}

	return a.PushFunc(ctx, bundle)
}
