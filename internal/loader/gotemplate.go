package loader

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"text/template"

	"github.com/go-sprout/sprout"
	"github.com/go-sprout/sprout/group/hermetic"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/kpm/internal/builder"
)

type GoTemplate struct {
	Registry *LoaderRegistry
	Template *template.Template
}

type GoTemplateData struct {
	Values map[string]any
}

func (l *GoTemplate) LoadSpecBytes(specFileData []byte, workingDir string, templateData GoTemplateData) (builder.Builder, error) {
	specTemplate, err := l.Template.Parse(string(specFileData))
	if err != nil {
		return nil, err
	}

	var specDataBuf bytes.Buffer
	if err := specTemplate.Execute(&specDataBuf, templateData); err != nil {
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

	return loadSpecFunc(specDataBuf.Bytes(), workingDir)
}

func (l *GoTemplate) LoadSpecFile(path string, templateData GoTemplateData) (builder.Builder, error) {
	specFileData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return l.LoadSpecBytes(specFileData, filepath.Dir(path), templateData)
}

var DefaultGoTemplate = &GoTemplate{
	Registry: DefaultRegistry,
	Template: template.New("").
		Funcs(sprout.New(sprout.WithGroups(hermetic.RegistryGroup())).Build()),
}
