package spec

import (
	"context"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"

	bundlev1alpha1 "github.com/joelanford/kpm/internal/api/bundle/v1alpha1"
)

type Spec interface {
	ID() bundlev1alpha1.ID
	MarshalOCI(context.Context, content.Pusher) (ocispec.Descriptor, error)
}
