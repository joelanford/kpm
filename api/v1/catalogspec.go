package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type CatalogSpec struct {
	metav1.TypeMeta `json:",inline"`

	DisplayName      string            `json:"displayName,omitempty"`
	Publisher        string            `json:"publisher,omitempty"`
	Description      string            `json:"description,omitempty"`
	ExtraAnnotations map[string]string `json:"annotations,omitempty"`

	Type string     `json:"type"`
	FBC  *FBCSource `json:"fbc,omitempty"`
}

type FBCSource struct {
	CatalogDir string `json:"catalogDir,omitempty"`
}

var DefaultFBCSpec = `---
apiVersion: kpm.io/v1
kind: CatalogSpec
type: fbc
fbc: {}
`
