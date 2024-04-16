package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BundleSpec struct {
	metav1.TypeMeta `json:",inline"`

	Type       string            `json:"type"`
	RegistryV1 *RegistryV1Source `json:"registryV1,omitempty"`
}

type RegistryV1Source struct {
	ManifestsDir string `json:"manifestsDir,omitempty"`
	MetadataDir  string `json:"metadataDir,omitempty"`
}

var DefaultRegistryV1Spec = `---
apiVersion: kpm.io/v1
kind: BundleSpec
type: registryV1
registryV1: {}
`
