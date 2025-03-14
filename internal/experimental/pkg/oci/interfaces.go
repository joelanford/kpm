package oci

import (
	"iter"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type Annotated interface {
	Annotations() map[string]string
}

type AnnotatedDescriptor interface {
	DescriptorAnnotations() map[string]string
}

type Artifact interface {
	ArtifactType() string
}

type ArtifactReferrer interface {
	Subject() Artifact
}

type ShallowReferrer interface {
	Subject() ocispec.Descriptor
}

type DeepArtifact interface {
	Artifact
	Config() Blob
	SubArtifacts() iter.Seq2[int, Artifact]
	Blobs() iter.Seq2[int, Blob]
}

type ShallowArtifact interface {
	Artifact
	Config() ocispec.Descriptor
	SubArtifacts() iter.Seq2[int, ocispec.Descriptor]
	Blobs() iter.Seq2[int, ocispec.Descriptor]
}

type Blob interface {
	MediaType() string
	Data() ([]byte, error)
}

type staticBlob struct {
	mediaType string
	data      []byte
}

func (s *staticBlob) MediaType() string {
	return s.mediaType
}

func (s *staticBlob) Data() ([]byte, error) {
	return s.data, nil
}

func EmptyJSONBlob() Blob {
	return BlobFromBytes(ocispec.MediaTypeEmptyJSON, []byte("{}"))
}

func BlobFromBytes(mediaType string, b []byte) Blob {
	return &staticBlob{mediaType: mediaType, data: b}
}
