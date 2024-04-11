package v1

import (
	"github.com/containerd/containerd/images"
	"github.com/joelanford/kpm/oci"
)

type DockerImage struct {
	configData  []byte
	blobData    []byte
	annotations map[string]string
	tag         string
}

func NewDockerImage(tag string, configData, blobData []byte, annotations map[string]string) *DockerImage {
	return &DockerImage{
		tag:         tag,
		configData:  configData,
		blobData:    blobData,
		annotations: annotations,
	}
}

func (l DockerImage) MediaType() string {
	return images.MediaTypeDockerSchema2Manifest
}

func (l DockerImage) ArtifactType() string {
	return ""
}

func (l DockerImage) Config() oci.Blob {
	return oci.BlobFromBytes(images.MediaTypeDockerSchema2Config, l.configData)
}

func (l DockerImage) Annotations() (map[string]string, error) {
	return l.annotations, nil
}

func (l DockerImage) SubArtifacts() []oci.Artifact {
	return nil
}

func (l DockerImage) Blobs() []oci.Blob {
	return []oci.Blob{oci.BlobFromBytes(images.MediaTypeDockerSchema2LayerGzip, l.blobData)}
}

func (l DockerImage) Tag() string {
	return l.tag
}
