package transport

import (
	"cmp"
	"context"

	kpmoci "github.com/joelanford/kpm/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
)

type ORASTarget struct {
	Remote oras.Target
	Tag    string
}

func (t *ORASTarget) Push(ctx context.Context, artifact kpmoci.Artifact) (string, ocispec.Descriptor, error) {
	desc, err := kpmoci.Push(ctx, artifact, t.Remote, kpmoci.PushOptions{})
	if err != nil {
		return "", ocispec.Descriptor{}, err
	}

	tag := cmp.Or(t.Tag, artifact.Tag())
	if tag != "" {
		if err := t.Remote.Tag(ctx, desc, tag); err != nil {
			return "", ocispec.Descriptor{}, err
		}
	}
	return tag, desc, nil
}
