package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	CatalogSpecSourceTypeFBC         = "fbc"
	CatalogSpecSourceTypeFBCTemplate = "fbcTemplate"

	KindCatalog = "Catalog"
)

type Catalog struct {
	metav1.TypeMeta `json:",inline"`

	Tag              string            `json:"tag"`
	Source           CatalogSpecSource `json:"source"`
	ExtraAnnotations map[string]string `json:"extraAnnotations,omitempty"`
}

type CatalogSpecSource struct {
	SourceType  string             `json:"sourceType"`
	FBC         *FBCSource         `json:"fbc,omitempty"`
	FBCTemplate *FBCTemplateSource `json:"fbcTemplate,omitempty"`
}

type FBCSource struct {
	CatalogRoot string `json:"catalogRoot"`
}

type FBCTemplateSource struct {
	TemplateFile   string `json:"templateFile"`
	MigrationLevel string `json:"migrationLevel,omitempty"`
}
