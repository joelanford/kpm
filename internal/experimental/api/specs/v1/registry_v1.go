package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	KindRegistryV1 = "RegistryV1"

	RegistryV1SourceTypeBundleDirectory = "BundleDirectory"
	RegistryV1SourceTypeManifests       = "Manifests"
	RegistryV1SourceTypeKustomization   = "Kustomization"
)

type RegistryV1 struct {
	metav1.TypeMeta `json:",inline"`

	Release string           `json:"release"`
	Source  RegistryV1Source `json:"source"`
}

type RegistryV1Source struct {
	SourceType      string                           `json:"sourceType"`
	BundleDirectory *RegistryV1BundleDirectorySource `json:"bundleDirectory,omitempty"`
	Manifests       *RegistryV1ManifestsSource       `json:"manifests,omitempty"`
	Kustomization   *RegistryV1KustomizationSource   `json:"kustomization,omitempty"`
}

type RegistryV1BundleDirectorySource struct {
	Path string `json:"path"`
}
type RegistryV1ManifestsSource struct {
	Paths []string `json:"paths"`
}
type RegistryV1KustomizationSource struct {
	Path string `json:"path"`
}
