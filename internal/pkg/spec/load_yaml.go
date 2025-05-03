package spec

import (
	"fmt"
	"os"
	"path/filepath"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

type YAML struct {
	Registry *Registry
}

func (l *YAML) LoadSpecFile(path string) (Spec, error) {
	specFileData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	var obj metav1.PartialObjectMetadata
	if err := yaml.Unmarshal(specFileData, &obj); err != nil {
		return nil, err
	}

	gvk := obj.GroupVersionKind()
	loadSpecFunc, err := l.Registry.GetLoadSpecFunc(gvk)
	if err != nil {
		return nil, fmt.Errorf("failed to get spec loader from registry for GVK %q: %w", gvk, err)
	}

	return loadSpecFunc(specFileData, filepath.Dir(path))
}

var DefaultYAML = &YAML{
	Registry: DefaultRegistry,
}
