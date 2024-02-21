package v1

import (
	"github.com/containerd/containerd/images"
	"github.com/joelanford/kpm/oci"
)

var _ oci.Artifact = (*LegacyRegistryBundle)(nil)

type LegacyRegistryBundle struct {
	name        string
	configData  []byte
	blobData    []byte
	annotations map[string]string
}

func NewLegacyRegistryBundle(name string, configData, blobData []byte, annotations map[string]string) *LegacyRegistryBundle {
	return &LegacyRegistryBundle{
		name:        name,
		configData:  configData,
		blobData:    blobData,
		annotations: annotations,
	}
}

func (l LegacyRegistryBundle) MediaType() string {
	return images.MediaTypeDockerSchema2Manifest
}

func (l LegacyRegistryBundle) ArtifactType() string {
	return ""
}

func (l LegacyRegistryBundle) Config() oci.Blob {
	return oci.BlobFromBytes(images.MediaTypeDockerSchema2Config, l.configData)
}

func (l LegacyRegistryBundle) Annotations() (map[string]string, error) {
	return l.annotations, nil
}

func (l LegacyRegistryBundle) SubArtifacts() []oci.Artifact {
	return nil
}

func (l LegacyRegistryBundle) Blobs() []oci.Blob {
	return []oci.Blob{oci.BlobFromBytes(images.MediaTypeDockerSchema2LayerGzip, l.blobData)}
}

func (l LegacyRegistryBundle) Tag() string {
	return l.name
}
