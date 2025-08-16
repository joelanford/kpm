package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	KindRegistryV1 = "RegistryV1"

	RegistryV1SourceTypeBundleDirectory = "BundleDirectory"
	RegistryV1SourceTypeGenerate        = "Generate"
)

type RegistryV1 struct {
	metav1.TypeMeta `json:",inline"`

	Source RegistryV1Source `json:"source"`
}

type RegistryV1Source struct {
	SourceType      string                           `json:"sourceType"`
	BundleDirectory *RegistryV1BundleDirectorySource `json:"bundleDirectory,omitempty"`
	Generate        *RegistryV1GenerateSource        `json:"generate,omitempty"`
}

type RegistryV1BundleDirectorySource struct {
	Path string `json:"path"`
}

type RegistryV1GenerateSource struct {
	// manifestFiles specifies the glob patterns of files in which plain
	// Kubernetes YAML manifests for the bundle exist.
	//
	// manifestFiles is expected to contain exactly one manifest with a base
	// ClusterServiceVersion. Some kinds of manifests found in manifestFiles are
	// used to populate fields in the CSV (e.g. Deployments, RBAC, and various
	// webhook configurations). Other supported kinds will be directly included
	// as manifests in the bundle.
	//
	// If manifestFiles contains manifests with unsupported kinds, according to the
	// registry+v1 spec, an error will be reported.
	ManifestFiles []string `json:"manifestFiles"`

	PackageName string                      `json:"packageName"`
	CSV         RegistryV1GenerateSourceCSV `json:"csv"`
}

type RegistryV1GenerateSourceCSV struct {
	Version              string            `json:"version"`
	Annotations          map[string]string `json:"annotations"`
	ExtraServiceAccounts []string          `json:"extraServiceAccounts"`
}
