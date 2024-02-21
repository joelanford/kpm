package action

import (
	"context"
	"io/fs"

	buildv1 "github.com/joelanford/kpm/build/v1"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type BuildLegacyRegistryBundle struct {
	RootFS   fs.FS
	PushFunc PushFunc
}

func (a *BuildLegacyRegistryBundle) Run(ctx context.Context) (string, ocispec.Descriptor, error) {
	bundle, err := buildv1.LegacyRegistryBundle(a.RootFS)
	if err != nil {
		return "", ocispec.Descriptor{}, err
	}

	return a.PushFunc(ctx, bundle)
}
