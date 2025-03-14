package v1

import (
	"encoding/json"
	"fmt"
	"iter"
	"strconv"

	"github.com/blang/semver/v4"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/joelanford/kpm/internal/experimental/pkg/oci"
)

const (
	ArtifactTypeBundle = "application/vnd.operatorframework.olm.bundle.v1+json"

	MediaTypeBundleIdentity           = "application/vnd.operatorframework.olm.bundle.identity.v1+json"
	MediaTypeBundleResolutionMetadata = "application/vnd.operatorframework.olm.bundle.metadata.resolution.v1+json"
	MediaTypeBundleQueryMetadata      = "application/vnd.operatorframework.olm.bundle.metadata.query.v1+json"
	MediaTypeBundleDisplayMetadata    = "application/vnd.operatorframework.olm.bundle.metadata.display.v1+json"
	MediaTypeBundleMirrorMetadata     = "application/vnd.operatorframework.olm.bundle.metadata.mirror.v1+json"
	MediaTypeBundleExtendedMetadata   = "application/vnd.operatorframework.olm.bundle.metadata.extended.v1+json"

	AnnotationBundleVersion = "olm.operatorframework.io/bundle-version"
	AnnotationBundleRelease = "olm.operatorframework.io/bundle-release"
)

var (
	_ oci.Artifact = (*Bundle)(nil)
	_ oci.Blob     = (*BundleIdentity)(nil)
	_ oci.Blob     = (*BundleResolutionMetadata)(nil)
	_ oci.Blob     = (*BundleQueryMetadata)(nil)
	_ oci.Blob     = (*BundleDisplayMetadata)(nil)
	_ oci.Blob     = (*BundleMirrorMetadata)(nil)
	_ oci.Blob     = (*BundleExtendedMetadata)(nil)
)

type Bundle struct {
	ID                 BundleIdentity            `oci:"config:mediaType=application/vnd.operatorframework.olm.bundle.identity.v1+json"`
	ResolutionMetadata *BundleResolutionMetadata `oci:"blob:mediaType=application/vnd.operatorframework.olm.bundle.metadata.resolution.v1+json"`
	QueryMetadata      *BundleQueryMetadata      `oci:"blob:mediaType=application/vnd.operatorframework.olm.bundle.metadata.query.v1+json"`
	DisplayMetadata    *BundleDisplayMetadata    `oci:"blob:mediaType=application/vnd.operatorframework.olm.bundle.metadata.display.v1+json"`
	MirrorMetadata     *BundleMirrorMetadata     `oci:"blob:mediaType=application/vnd.operatorframework.olm.bundle.metadata.mirror.v1+json"`
	ExtendedMetadata   *BundleExtendedMetadata   `oci:"blob:mediaType=application/vnd.operatorframework.olm.bundle.extended.v1+json"`
}

func (b Bundle) ArtifactType() string {
	return ArtifactTypeBundle
}

func (b Bundle) Config() oci.Blob {
	return &b.ID
}

func (b Bundle) SubArtifacts() iter.Seq2[int, oci.Artifact] {
	return func(yield func(int, oci.Artifact) bool) {}
}

func (b Bundle) Blobs() iter.Seq2[int, oci.Blob] {
	return func(yield func(int, oci.Blob) bool) {
		i := 0
		if b.ResolutionMetadata != nil && !yield(i, b.ResolutionMetadata) {
			return
		}
		i++
		if b.QueryMetadata != nil && !yield(i, b.QueryMetadata) {
			return
		}
		i++
		if b.DisplayMetadata != nil && !yield(i, b.DisplayMetadata) {
			return
		}
		i++
		if b.MirrorMetadata != nil && !yield(i, b.MirrorMetadata) {
			return
		}
		i++
		if b.ExtendedMetadata != nil && !yield(i, b.ExtendedMetadata) {
			return
		}
	}
}

func (b Bundle) Annotations() map[string]string {
	return map[string]string{
		AnnotationPackageName:   b.ID.Package,
		AnnotationBundleVersion: b.ID.Version.String(),
		AnnotationBundleRelease: strconv.Itoa(b.ID.Release),
	}
}

type BundleIdentity struct {
	Package string         `json:"package"`
	Version semver.Version `json:"version"`
	Release int            `json:"release"`
	Aliases []string       `json:"aliases,omitempty"`

	URI string `json:"uri"`
}

func (b *BundleIdentity) String() string {
	return fmt.Sprintf("%s.v%s-%d", b.Package, b.Version, b.Release)
}
func (b *BundleIdentity) MediaType() string     { return MediaTypeBundleIdentity }
func (b *BundleIdentity) Data() ([]byte, error) { return json.Marshal(b) }

type BundleResolutionMetadata struct {
	ProvidedGVKs           []schema.GroupVersionKind `json:"providedGVKs,omitempty"`
	RequiredGVKs           []schema.GroupVersionKind `json:"requiredGVKs,omitempty"`
	RequiredPackages       []RequiredPackage         `json:"requiredPackages,omitempty"`
	KubernetesVersionRange string                    `json:"kubernetesVersionRange,omitempty"`
}

type RequiredPackage struct {
	Name         string `json:"name"`
	VersionRange string `json:"versionRange,omitempty"`
}

func (b *BundleResolutionMetadata) MediaType() string     { return MediaTypeBundleResolutionMetadata }
func (b *BundleResolutionMetadata) Data() ([]byte, error) { return json.Marshal(b) }

type BundleQueryMetadata struct {
	Keywords []string `json:"keywords,omitempty"`
	Maturity string   `json:"maturity,omitempty"`
	Provider NamedURL `json:"provider,omitempty"`
}

func (b *BundleQueryMetadata) MediaType() string     { return MediaTypeBundleQueryMetadata }
func (b *BundleQueryMetadata) Data() ([]byte, error) { return json.Marshal(b) }

type BundleDisplayMetadata struct {
	Description string       `json:"description,omitempty"`
	Links       []NamedURL   `json:"links,omitempty"`
	Maintainers []NamedEmail `json:"maintainers,omitempty"`
}

func (b *BundleDisplayMetadata) MediaType() string     { return MediaTypeBundleDisplayMetadata }
func (b *BundleDisplayMetadata) Data() ([]byte, error) { return json.Marshal(b) }

type BundleMirrorMetadata struct {
	RelatedImages []string `json:"relatedImages"`
}

func (b *BundleMirrorMetadata) MediaType() string     { return MediaTypeBundleMirrorMetadata }
func (b *BundleMirrorMetadata) Data() ([]byte, error) { return json.Marshal(b) }

type BundleExtendedMetadata struct {
	Annotations map[string]string `json:"annotations"`
}

func (b *BundleExtendedMetadata) MediaType() string     { return MediaTypeBundleExtendedMetadata }
func (b *BundleExtendedMetadata) Data() ([]byte, error) { return json.Marshal(b) }
