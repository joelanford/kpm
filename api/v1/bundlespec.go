package v1

import (
	"bytes"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BundleSpec struct {
	metav1.TypeMeta `json:",inline"`

	Type       string            `json:"type"`
	RegistryV1 *RegistryV1Source `json:"registryV1,omitempty"`
	Bundle     *BundleSource     `json:"bundle,omitempty"`
}

type RegistryV1Source struct {
	ManifestsDir string `json:"manifestsDir,omitempty"`
	MetadataDir  string `json:"metadataDir,omitempty"`
}

type BundleSource struct {
	BundleConfig `json:",inline"`

	Source BundleSourceSource `json:"source"`

	Annotations map[string]string `json:"annotations,omitempty"`
}

type BundleSourceSource struct {
	Type string            `json:"type"`
	File *BundleSourceFile `json:"file,omitempty"`
	Dir  *BundleSourceDir  `json:"dir,omitempty"`

	MediaType string `json:"mediaType"`
}

type BundleSourceFile struct {
	Path string `json:"path"`
}

type BundleSourceDir struct {
	Path        string `json:"path"`
	Archive     string `json:"archive"`
	Compression string `json:"compression,omitempty"`
}

var DefaultRegistryV1Spec = bytes.NewReader([]byte(`---
apiVersion: kpm.io/v1
kind: BundleSpec
type: registryV1
registryV1: {}
`))
