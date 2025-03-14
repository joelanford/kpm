package v1

import (
	"iter"

	"github.com/joelanford/kpm/internal/experimental/pkg/oci"
)

const (
	ArtifactTypeCatalog = "application/vnd.operatorframework.olm.catalog.v1+json"
)

var _ oci.Artifact = (*Catalog)(nil)

type Catalog struct {
	Packages []Package `oci:"artifact:artifactType=application/vnd.operatorframework.olm.package.v1+json"`
}

func (c *Catalog) ArtifactType() string {
	return ArtifactTypeCatalog
}

func (c *Catalog) Config() oci.Blob {
	return nil
}

func (c *Catalog) SubArtifacts() iter.Seq2[int, oci.Artifact] {
	return func(yield func(int, oci.Artifact) bool) {
		for i, pkg := range c.Packages {
			if !yield(i, &pkg) {
				return
			}
		}
	}
}

func (c *Catalog) Blobs() iter.Seq2[int, oci.Blob] {
	return func(yield func(int, oci.Blob) bool) {}
}
