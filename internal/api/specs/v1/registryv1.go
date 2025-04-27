package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	KindRegistryV1 = "RegistryV1"

	RegistryV1SourceTypeBundleDirectory = "BundleDirectory"
)

type RegistryV1 struct {
	metav1.TypeMeta `json:",inline"`

	Release          string            `json:"release"`
	ExtraAnnotations map[string]string `json:"extraAnnotations"`
	Source           RegistryV1Source  `json:"source"`
}

type RegistryV1Source struct {
	SourceType      string                           `json:"sourceType"`
	BundleDirectory *RegistryV1BundleDirectorySource `json:"bundleDirectory,omitempty"`
}

type RegistryV1BundleDirectorySource struct {
	Path string `json:"path"`
}
