package v1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/joelanford/kpm/internal/maps"
	"github.com/joelanford/kpm/oci"
)

const (
	ArtifactTypeBundle    = "application/vnd.io.kpm.bundle.v1+json"
	MediaTypeBundleConfig = "application/vnd.io.kpm.bundle.config.v1+json"
)

var _ oci.Artifact = (*Bundle)(nil)

type Bundle struct {
	BundleConfig
	BundleContent

	ExtraAnnotations map[string]string

	// TODO: handle related images
}

type BundleConfig struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Release string `json:"release"`

	Provides  []string `json:"provides,omitempty"`
	Requires  []string `json:"requires,omitempty"`
	Conflicts []string `json:"conflicts,omitempty"`
}

type BundleContent struct {
	ContentMediaType string
	Content          []byte
}

func (b *Bundle) ArtifactType() string {
	return ArtifactTypeBundle
}

func (b *Bundle) Config() oci.Blob {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(b.BundleConfig); err != nil {
		panic(err)
	}
	return oci.BlobFromBytes(MediaTypeBundleConfig, bytes.TrimSpace(buf.Bytes()))
}

func (b *Bundle) Annotations() (map[string]string, error) {
	return maps.MergeStrict(map[string]string{
		"kpm.io/name":    b.Name,
		"kpm.io/version": b.Version,
		"kpm.io/release": b.Release,
	}, b.ExtraAnnotations)
}

func (b *Bundle) SubArtifacts() []oci.Artifact {
	return nil
}

func (b *Bundle) Blobs() []oci.Blob {
	return []oci.Blob{oci.BlobFromBytes(b.ContentMediaType, b.Content)}
}

func (b *Bundle) String() string {
	return fmt.Sprintf("%s-%s-%s", b.Name, b.Version, b.Release)
}

func (b *Bundle) Validate() error {
	var errs []error
	if b.Name == "" {
		errs = append(errs, fmt.Errorf("name is required"))
	} else if verrs := validation.IsDNS1123Label(b.Name); len(verrs) != 0 {
		errs = append(errs, fmt.Errorf("name is invalid: %s", strings.Join(verrs, ", ")))
	}

	if b.Version == "" {
		errs = append(errs, fmt.Errorf("version is required"))
	} else if _, err := semver.StrictNewVersion(b.Version); err != nil {
		errs = append(errs, fmt.Errorf("version is invalid: %w", err))
	}

	if b.Release == "" {
		errs = append(errs, fmt.Errorf("release is required"))
	} else if verrs := validation.IsDNS1123Label(b.Release); len(verrs) != 0 {
		errs = append(errs, fmt.Errorf("release is invalid: %s", strings.Join(verrs, ", ")))
	}

	if _, err := b.Annotations(); err != nil {
		errs = append(errs, fmt.Errorf("invalid extra annotations: %w", err))
	}

	return errors.Join(errs...)
}
