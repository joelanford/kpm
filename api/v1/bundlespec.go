package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

type BundleSpec struct {
	metav1.TypeMeta `json:",inline"`
	BundleConfig    `json:",inline"`
	Source          *BundleSource     `json:"source,omitempty"`
	Annotations     map[string]string `json:"annotations,omitempty"`
}

type BundleSource struct {
	Type string            `json:"type"`
	File *BundleSourceFile `json:"file,omitempty"`

	MediaType string `json:"mediaType"`
}

type BundleSourceFile struct {
	Path string `json:"path"`
}
