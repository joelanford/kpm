package v1

import (
	"github.com/joelanford/kpm/internal/experimental/pkg/oci"
)

const (
	ArtifactTypeDeprecation = "application/vnd.operatorframework.olm.deprecation.v1"

	AnnotationDeprecationMessage = "olm.operatorframework.io/deprecation-message"
)

var (
	_ oci.Artifact = (*Deprecation[oci.Artifact])(nil)
)

type Deprecation[T oci.Artifact] struct {
	Message   string `oci:"annotation:key=olm.operatorframework.io/deprecation-message"`
	Reference T      `oci:"subject:mediaType=application/vnd.oci.image.manifest.v1+json"`
}

func (p *Deprecation[T]) ArtifactType() string {
	return ArtifactTypeDeprecation
}

func (p *Deprecation[T]) Subject() oci.Artifact {
	return p.Reference
}

func (p *Deprecation[T]) Annotations() map[string]string {
	return map[string]string{
		AnnotationDeprecationMessage: p.Message,
	}
}
