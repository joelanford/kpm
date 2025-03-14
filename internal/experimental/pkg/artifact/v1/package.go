package v1

import (
	"encoding/json"
	"iter"

	"github.com/joelanford/kpm/internal/experimental/pkg/oci"
)

const (
	ArtifactTypePackage             = "application/vnd.operatorframework.olm.package.v1+json"
	MediaTypePackageIdentity        = "application/vnd.operatorframework.olm.package.identity.v1+json"
	MediaTypePackageDisplayMetadata = "application/vnd.operatorframework.olm.package.metadata.display.v1+json"
	AnnotationPackageDefaultChannel = "olm.operatorframework.io/default-channel"
)

var (
	_ oci.Artifact = (*Package)(nil)
	_ oci.Blob     = (*PackageIdentity)(nil)
)

type Package struct {
	ID              PackageIdentity         `oci:"config:mediaType=application/vnd.operatorframework.olm.package.identity.v1+json"`
	DisplayMetadata *PackageDisplayMetadata `oci:"blob:mediaType=application/vnd.operatorframework.olm.package.metadata.display.v1+json"`
	Icon            *Icon                   `oci:"blob:selector=olm.operatorframework.io/is-icon"`
	Channels        []Channel               `oci:"artifact:artifactType=application/vnd.operatorframework.olm.channel.v1+json"`

	// DefaultChannel is the default channel of the package.
	// This field is required in catalogs used by OLMv0.
	// This field is ignored in catalogs used by OLMv1.
	DefaultChannel string `oci:"annotation:key=olm.operatorframework.io/default-channel"`
}

func (p *Package) ArtifactType() string {
	return ArtifactTypePackage
}

func (p *Package) Config() oci.Blob {
	return &p.ID
}

func (p *Package) SubArtifacts() iter.Seq2[int, oci.Artifact] {
	return func(yield func(int, oci.Artifact) bool) {
		for i, ch := range p.Channels {
			if !yield(i, &ch) {
				return
			}
		}
	}
}

func (p *Package) Blobs() iter.Seq2[int, oci.Blob] {
	return func(yield func(int, oci.Blob) bool) {
		i := 0
		if p.DisplayMetadata != nil && !yield(i, p.DisplayMetadata) {
			return
		}
		i++
		if p.Icon != nil && !yield(i, p.Icon) {
			return
		}
		i++
	}
}

func (p *Package) Annotations() map[string]string {
	m := map[string]string{
		AnnotationPackageName: p.ID.Name,
	}
	if p.DefaultChannel != "" {
		m[AnnotationPackageDefaultChannel] = p.DefaultChannel
	}
	return m
}

type PackageIdentity struct {
	Name string `json:"name"`
}

func (p *PackageIdentity) String() string {
	return p.Name
}
func (p *PackageIdentity) MediaType() string     { return MediaTypePackageIdentity }
func (p *PackageIdentity) Data() ([]byte, error) { return json.Marshal(p) }

type PackageDisplayMetadata struct {
	DisplayName string `json:"displayName,omitempty"`
	Description string `json:"description,omitempty"`
}

func (p *PackageDisplayMetadata) MediaType() string     { return MediaTypePackageDisplayMetadata }
func (p *PackageDisplayMetadata) Data() ([]byte, error) { return json.Marshal(p) }
