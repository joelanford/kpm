package spec

import (
	"context"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
)

type Spec interface {
	ID() string
	MarshalOCI(context.Context, oras.Target) (ocispec.Descriptor, error)
}
