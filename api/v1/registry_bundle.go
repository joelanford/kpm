package v1

import (
	"github.com/containerd/containerd/images"
	"github.com/joelanford/kpm/oci"
)

var _ oci.Artifact = (*RegistryBundle)(nil)

type RegistryBundle struct {
	tag         string
	configData  []byte
	blobData    []byte
	annotations map[string]string
}

func NewRegistryBundle(tag string, configData, blobData []byte, annotations map[string]string) *RegistryBundle {
	return &RegistryBundle{
		tag:         tag,
		configData:  configData,
		blobData:    blobData,
		annotations: annotations,
	}
}

func (l RegistryBundle) MediaType() string {
	return images.MediaTypeDockerSchema2Manifest
}

func (l RegistryBundle) ArtifactType() string {
	return ""
}

func (l RegistryBundle) Config() oci.Blob {
	return oci.BlobFromBytes(images.MediaTypeDockerSchema2Config, l.configData)
}

func (l RegistryBundle) Annotations() (map[string]string, error) {
	return l.annotations, nil
}

func (l RegistryBundle) SubArtifacts() []oci.Artifact {
	return nil
}

func (l RegistryBundle) Blobs() []oci.Blob {
	return []oci.Blob{oci.BlobFromBytes(images.MediaTypeDockerSchema2LayerGzip, l.blobData)}
}

func (l RegistryBundle) Tag() string {
	return l.tag
}
