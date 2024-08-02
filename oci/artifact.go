package oci

import (
	"bytes"
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type Artifact interface {
	MediaTyper
	ArtifactType() string
	Config() Blob
	Annotations() (map[string]string, error)
	Subject() *ocispec.Descriptor
	SubArtifacts() []Artifact
	Blobs() []Blob
	Tag() string
}

type MediaTyper interface {
	MediaType() string
}

type Blob interface {
	MediaTyper
	Data() (io.ReadCloser, error)
}

func BlobFromBytes(mediaType string, data []byte) Blob {
	return &byteBlob{
		mediaType: mediaType,
		data:      bytes.NewReader(data),
	}
}

type byteBlob struct {
	mediaType string
	data      *bytes.Reader
}

func (b *byteBlob) MediaType() string {
	return b.mediaType
}

func (b *byteBlob) Data() (io.ReadCloser, error) {
	return io.NopCloser(b.data), nil
}
