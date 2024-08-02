package v1

import (
	"github.com/containers/image/v5/docker/reference"
	"github.com/joelanford/kpm/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

const (
	AnnotationOriginReference   = "io.kpm.origin.reference"
	ArtifactTypeOriginReference = "application/vnd.kpm.origin-reference.v1+json"
)

type OriginReference struct {
	subject ocispec.Descriptor
	ref     reference.Named
}

func NewOriginReference(subject ocispec.Descriptor, ref reference.Named) *OriginReference {
	return &OriginReference{
		subject: subject,
		ref:     ref,
	}
}

func (l OriginReference) MediaType() string {
	return ocispec.MediaTypeImageManifest
}

func (l OriginReference) ArtifactType() string {
	return ArtifactTypeOriginReference
}

func (l OriginReference) Config() oci.Blob {
	return oci.BlobFromBytes(ocispec.MediaTypeEmptyJSON, []byte("{}"))
}

func (l OriginReference) Annotations() (map[string]string, error) {
	return map[string]string{
		AnnotationOriginReference: l.ref.String(),
	}, nil
}

func (l OriginReference) SubArtifacts() []oci.Artifact {
	return nil
}

func (l OriginReference) Blobs() []oci.Blob {
	return nil
}

func (l OriginReference) Subject() *ocispec.Descriptor {
	return &l.subject
}

func (l OriginReference) Tag() string {
	return ""
}
