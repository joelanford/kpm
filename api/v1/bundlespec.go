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
