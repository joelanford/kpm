package spec

import (
	"fmt"
	"os"
	"path/filepath"

	"sigs.k8s.io/yaml"

	specsv1 "github.com/joelanford/kpm/internal/api/specs/v1"
	registryv1 "github.com/joelanford/kpm/internal/pkg/bundle/registry/v1"
)

func init() {
	if err := DefaultRegistry.RegisterKind(specsv1.GroupVersion.WithKind(specsv1.KindRegistryV1), loadRegistryV1Bytes); err != nil {
		panic(err)
	}
}

func loadRegistryV1Bytes(specData []byte, workingDir string) (Spec, error) {
	var rv1Spec specsv1.RegistryV1
	if err := yaml.Unmarshal(specData, &rv1Spec); err != nil {
		return nil, err
	}
	return loadRegistryV1(rv1Spec, workingDir)
}

func loadRegistryV1(spec specsv1.RegistryV1, workingDir string) (Spec, error) {
	switch spec.Source.SourceType {
	case specsv1.RegistryV1SourceTypeBundleDirectory:
		return registryv1.LoadFS(os.DirFS(filepath.Join(workingDir, spec.Source.BundleDirectory.Path)))
	default:
		return nil, fmt.Errorf("unknown source type: %q", spec.Source.SourceType)
	}
}
