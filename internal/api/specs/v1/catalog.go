package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CatalogSpecSourceTypeBundles       = "bundles"
	CatalogSpecSourceTypeFBC           = "fbc"
	CatalogSpecSourceTypeFBCTemplate   = "fbcTemplate"
	CatalogSpecSourceTypeFBCGoTemplate = "fbcGoTemplate"
	CatalogSpecSourceTypeLegacy        = "legacy"

	KindCatalog = "Catalog"
)

type Catalog struct {
	metav1.TypeMeta `json:",inline"`

	RegistryNamespace string `json:"registryNamespace"`
	Name              string `json:"name"`
	Tag               string `json:"tag"`

	MigrationLevel string `json:"migrationLevel,omitempty"`
	CacheFormat    string `json:"cacheFormat,omitempty"`

	Source CatalogSpecSource `json:"source"`

	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
}

type CatalogSpecSource struct {
	SourceType    string               `json:"sourceType"`
	Bundles       *BundleSource        `json:"bundles,omitempty"`
	FBC           *FBCSource           `json:"fbc,omitempty"`
	FBCTemplate   *FBCTemplateSource   `json:"fbcTemplate,omitempty"`
	FBCGoTemplate *FBCGoTemplateSource `json:"fbcGoTemplate,omitempty"`
	Legacy        *LegacySource        `json:"legacy,omitempty"`
}

type BundleSource struct {
	BundleRoot string `json:"bundleRoot"`
}

type FBCSource struct {
	CatalogRoot string `json:"catalogRoot"`
}

type FBCTemplateSource struct {
	TemplateFile string `json:"templateFile"`
}

type FBCGoTemplateSource struct {
	BundleSpecGlobs     []string `json:"bundleSpecGlobs"`
	ValuesFiles         []string `json:"valuesFiles"`
	TemplateFile        string   `json:"templateFile"`
	TemplateHelperGlobs []string `json:"templateHelperGlobs"`
}

type LegacySource struct {
	BundleRoot              string `json:"bundleRoot"`
	BundleRegistryNamespace string `json:"bundleRegistryNamespace"`
}
