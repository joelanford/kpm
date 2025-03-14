package v1

import (
	"encoding/json"
	"fmt"
	"iter"

	"github.com/joelanford/kpm/internal/experimental/pkg/oci"
)

const (
	ArtifactTypeChannel = "application/vnd.operatorframework.olm.channel.v1+json"

	MediaTypeChannelIdentity = "application/vnd.operatorframework.olm.channel.identity.v1+json"

	AnnotationChannelName = "olm.operatorframework.io/channel-name"
)

var (
	_ oci.Artifact = (*Channel)(nil)
	_ oci.Blob     = (*ChannelIdentity)(nil)
)

type Channel struct {
	ID           ChannelIdentity `oci:"config:mediaType=application/vnd.operatorframework.olm.channel.identity.v1+json"`
	Bundles      []Bundle        `oci:"artifact:artifactType=application/vnd.operatorframework.olm.bundle.v1+json"`
	UpgradeEdges []UpgradeEdge   `oci:"artifact:artifactType=application/vnd.operatorframework.olm.upgrade.edge.v1+json"`
}

func (c Channel) ArtifactType() string {
	return ArtifactTypeChannel
}

func (c Channel) Config() oci.Blob {
	return &c.ID
}

func (c Channel) SubArtifacts() iter.Seq2[int, oci.Artifact] {
	return func(yield func(int, oci.Artifact) bool) {
		i := 0
		for _, b := range c.Bundles {
			if !yield(i, &b) {
				return
			}
			i++
		}
		for _, u := range c.UpgradeEdges {
			if !yield(i, u) {
				return
			}
			i++
		}
	}
}

func (c Channel) Blobs() iter.Seq2[int, oci.Blob] {
	return func(yield func(int, oci.Blob) bool) {}
}

func (c Channel) Annotations() map[string]string {
	return map[string]string{
		AnnotationPackageName: c.ID.Package,
		AnnotationChannelName: c.ID.Name,
	}
}

type ChannelIdentity struct {
	Package string `json:"package"`
	Name    string `json:"name"`
}

func (c *ChannelIdentity) String() string {
	return fmt.Sprintf("%s.%s", c.Package, c.Name)
}
func (c *ChannelIdentity) MediaType() string     { return MediaTypeChannelIdentity }
func (c *ChannelIdentity) Data() ([]byte, error) { return json.Marshal(c) }
