package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	KindBundle = "Bundle"
)

type Bundle struct {
	metav1.TypeMeta `json:",inline"`

	MediaType         string            `json:"mediaType"`
	RegistryNamespace string            `json:"registryNamespace"`
	BundleRoot        string            `json:"bundleRoot"`
	ExtraAnnotations  map[string]string `json:"extraAnnotations,omitempty"`
}
