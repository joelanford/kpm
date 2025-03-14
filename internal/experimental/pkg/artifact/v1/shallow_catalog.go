package v1

import (
	"iter"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/joelanford/kpm/internal/experimental/pkg/oci"
)

var _ oci.Artifact = (*ShallowCatalog)(nil)
var _ oci.ShallowArtifact = (*ShallowCatalog)(nil)

type ShallowCatalog struct {
	Packages []ocispec.Descriptor `oci:"blob:artifactType=application/vnd.operatorframework.olm.package.v1+json"`
}

func (c *ShallowCatalog) ArtifactType() string {
	return ArtifactTypeCatalog
}

func (c *ShallowCatalog) Config() ocispec.Descriptor {
	return ocispec.DescriptorEmptyJSON
}

func (c *ShallowCatalog) SubArtifacts() iter.Seq2[int, ocispec.Descriptor] {
	return func(yield func(int, ocispec.Descriptor) bool) {
		for i, pkgDesc := range c.Packages {
			if !yield(i, pkgDesc) {
				return
			}
		}
	}
}

func (c *ShallowCatalog) Blobs() iter.Seq2[int, ocispec.Descriptor] {
	return func(yield func(int, ocispec.Descriptor) bool) {}
}
