package v1

import (
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/joelanford/kpm/internal/experimental/pkg/oci"
)

const (
	AnnotationIsIcon = "olm.operatorframework.io/is-icon"
)

var _ oci.Blob = (*Icon)(nil)

type Icon struct {
	oci.Blob
}

func NewIcon(mediaType string, data []byte) *Icon {
	return &Icon{
		Blob: oci.BlobFromBytes(mediaType, data),
	}
}

func (i *Icon) Annotations() map[string]string {
	return map[string]string{
		AnnotationIsIcon: "true",
	}
}

func (i *Icon) UnmarshalBlob(descriptor ocispec.Descriptor, data []byte) error {
	i.Blob = oci.BlobFromBytes(descriptor.MediaType, data)
	return nil
}
