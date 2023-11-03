package v1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/joelanford/kpm/internal/maps"
	"github.com/joelanford/kpm/oci"
)

const (
	ArtifactTypePackage    = "application/vnd.io.kpm.package.v1+json"
	MediaTypePackageConfig = "application/vnd.io.kpm.package.config.v1+json"
)

var _ oci.Artifact = (*Package)(nil)

type Package struct {
	PackageConfig
	LongDescription  string
	Icon             *Icon
	ExtraAnnotations map[string]string

	Bundles []Bundle
}

func (c Package) ArtifactType() string {
	return ArtifactTypePackage
}

func (c Package) Config() oci.Blob {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(c.PackageConfig); err != nil {
		panic(err)
	}
	return oci.BlobFromBytes(MediaTypePackageConfig, bytes.TrimSpace(buf.Bytes()))
}

func (c Package) Annotations() (map[string]string, error) {
	return maps.MergeStrict(map[string]string{
		"kpm.io/name":             c.Name,
		"kpm.io/displayName":      c.DisplayName,
		"kpm.io/providerName":     c.ProviderName,
		"kpm.io/shortDescription": c.ShortDescription,
	}, c.ExtraAnnotations)
}

func (c Package) SubArtifacts() []oci.Artifact {
	var artifacts []oci.Artifact
	for _, b := range c.Bundles {
		b := b
		artifacts = append(artifacts, &b)
	}
	return artifacts
}

func (c Package) Blobs() []oci.Blob {
	var blobs []oci.Blob
	if c.Icon != nil {
		blobs = append(blobs, oci.BlobFromBytes(c.Icon.MediaType, c.Icon.Data))
	}
	return blobs
}

type PackageConfig struct {
	Name         string `json:"name"`
	DisplayName  string `json:"displayName"`
	ProviderName string `json:"providerName"`

	ShortDescription string `json:"shortDescription"`

	Maintainers []Maintainer `json:"maintainers"`
	Categories  []string     `json:"categories"`
}

type Maintainer struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (c Package) Validate() error {
	var errs []error
	if c.Name == "" {
		errs = append(errs, fmt.Errorf("name is required"))
	} else if verrs := validation.IsDNS1123Label(c.Name); len(verrs) != 0 {
		errs = append(errs, fmt.Errorf("name is invalid: %s", strings.Join(verrs, ", ")))
	}

	if c.DisplayName == "" {
		errs = append(errs, fmt.Errorf("displayName is required"))
	}

	if c.ShortDescription == "" {
		errs = append(errs, fmt.Errorf("shortDescription is required"))
	}
	if len(c.ShortDescription) > 140 {
		errs = append(errs, fmt.Errorf("shortDescription is too long, must be no more than 140 characters"))
	}

	if c.ProviderName == "" {
		errs = append(errs, fmt.Errorf("providerName is required"))
	}

	if len(c.Maintainers) == 0 {
		errs = append(errs, fmt.Errorf("at least one maintainer is required"))
	}
	for i, m := range c.Maintainers {
		if m.Email == "" {
			errs = append(errs, fmt.Errorf("maintainer[%d] email is required", i))
		}
	}

	for _, c := range c.Categories {
		if verrs := validation.IsDNS1035Label(c); len(verrs) != 0 {
			errs = append(errs, fmt.Errorf("category is invalid: %s", strings.Join(verrs, ", ")))
		}
	}

	if _, err := c.Annotations(); err != nil {
		errs = append(errs, fmt.Errorf("invalid extra annotations: %w", err))
	}

	if len(c.Bundles) == 0 {
		errs = append(errs, fmt.Errorf("at least one bundle is required"))
	}
	bundleIDs := map[string]int{}
	for i, b := range c.Bundles {
		bundleIDs[b.String()]++
		if b.Name != c.Name {
			errs = append(errs, fmt.Errorf("bundle[%d] is invalid: bundle name %q must match package name %q", i, b.Name, c.Name))
		}
		if err := b.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("bundle[%d] is invalid: %v", i, err))
		}
	}
	for id, count := range bundleIDs {
		if count > 1 {
			errs = append(errs, fmt.Errorf("duplicate bundle %q", id))
		}
	}

	return errors.Join(errs...)
}
