package spec

import (
	"fmt"
	"path/filepath"

	"sigs.k8s.io/yaml"

	specsv1 "github.com/joelanford/kpm/internal/api/specs/v1"
	helmv2 "github.com/joelanford/kpm/internal/bundle/helm/v2"
)

func init() {
	if err := DefaultRegistry.RegisterKind(specsv1.GroupVersion.WithKind(specsv1.KindHelmV2), loadHelmV2Bytes); err != nil {
		panic(err)
	}
}

func loadHelmV2Bytes(specData []byte, workingDir string) (Spec, error) {
	var rv1Spec specsv1.HelmV2
	if err := yaml.Unmarshal(specData, &rv1Spec); err != nil {
		return nil, err
	}
	return loadHelmV2(rv1Spec, workingDir)
}

func loadHelmV2(spec specsv1.HelmV2, workingDir string) (Spec, error) {
	switch spec.Source.SourceType {
	case specsv1.HelmV2SourceTypeBundleDirectory:
		return helmv2.LoadPackage(filepath.Join(workingDir, spec.Source.ChartArchive.ArchivePath))
	default:
		return nil, fmt.Errorf("unknown source type: %q", spec.Source.SourceType)
	}
}
