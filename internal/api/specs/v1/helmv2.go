package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	KindHelmV2 = "HelmV2"

	HelmV2SourceTypeBundleDirectory = "ChartArchive"
)

type HelmV2 struct {
	metav1.TypeMeta `json:",inline"`

	Source HelmV2Source `json:"source"`
}

type HelmV2Source struct {
	SourceType   string                    `json:"sourceType"`
	ChartArchive *HelmV2ChartArchiveSource `json:"chartArchive,omitempty"`
}

type HelmV2ChartArchiveSource struct {
	ArchivePath string `json:"archivePath"`
}
