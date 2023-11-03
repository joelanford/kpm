package v1

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/joelanford/kpm/internal/maps"
	"github.com/joelanford/kpm/oci"
)

const (
	ArtifactTypeCatalog    = "application/vnd.io.kpm.olm.catalog.v1"
	MediaTypeCatalogConfig = "application/vnd.io.kpm.catalog.config.v1+json"
)

type Catalog struct {
	CatalogConfig
	Icon     *Icon
	Packages []Package

	ExtraAnnotations map[string]string
}

type CatalogConfig struct {
	DisplayName  string `json:"displayName"`
	ProviderName string `json:"providerName"`

	ShortDescription string `json:"shortDescription"`
}

func (c Catalog) ArtifactType() string {
	return ArtifactTypeCatalog
}

func (c Catalog) Config() oci.Blob {
	buf := &bytes.Buffer{}
	enc := json.NewEncoder(buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(c.CatalogConfig); err != nil {
		panic(err)
	}
	return oci.BlobFromBytes(MediaTypeCatalogConfig, bytes.TrimSpace(buf.Bytes()))
}

func (c Catalog) Annotations() (map[string]string, error) {
	return maps.MergeStrict(map[string]string{
		"kpm.io/displayName":      c.DisplayName,
		"kpm.io/providerName":     c.ProviderName,
		"kpm.io/shortDescription": c.ShortDescription,
	}, c.ExtraAnnotations)
}

func (c Catalog) SubArtifacts() []oci.Artifact {
	var artifacts []oci.Artifact
	for _, p := range c.Packages {
		artifacts = append(artifacts, p)
	}
	return artifacts
}

func (c Catalog) Blobs() []oci.Blob {
	var blobs []oci.Blob
	if c.Icon != nil {
		blobs = append(blobs, oci.BlobFromBytes(c.Icon.MediaType, c.Icon.Data))
	}
	return blobs
}

func (c Catalog) Validate() error {
	var errs []error

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

	if _, err := c.Annotations(); err != nil {
		errs = append(errs, fmt.Errorf("invalid extra annotations: %w", err))
	}

	if len(c.Packages) == 0 {
		errs = append(errs, fmt.Errorf("at least one package is required"))
	}
	packageNames := map[string]int{}
	for i, p := range c.Packages {
		packageNames[p.Name]++
		if err := p.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("package[%d] is invalid: %v", i, err))
		}
	}
	for id, count := range packageNames {
		if count > 1 {
			errs = append(errs, fmt.Errorf("duplicate package %q", id))
		}
	}

	return errors.Join(errs...)
}
