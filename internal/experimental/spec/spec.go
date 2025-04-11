package spec

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"text/template"

	"github.com/containers/image/v5/docker/reference"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/yaml"
)

type Spec interface {
	Build(ref reference.NamedTagged, outputDir string) error
}

var (
	ErrKindNotRegistered     = fmt.Errorf("kind is not registered")
	ErrKindAlreadyRegistered = fmt.Errorf("kind is already registered")
)

type LoaderRegistry struct {
	reg map[schema.GroupVersionKind]LoadSpecBytesFunc
	mu  sync.RWMutex
}

func NewLoaderRegistry() LoaderRegistry {
	return LoaderRegistry{reg: make(map[schema.GroupVersionKind]LoadSpecBytesFunc)}
}

type LoadSpecBytesFunc func([]byte, string, map[string]string) (Spec, error)

func (r *LoaderRegistry) RegisterKind(gvk schema.GroupVersionKind, loadSpecFunc LoadSpecBytesFunc) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.reg[gvk]; exists {
		return ErrKindAlreadyRegistered
	}
	r.reg[gvk] = loadSpecFunc
	return nil
}

func (r *LoaderRegistry) GetLoadSpecFunc(gvk schema.GroupVersionKind) (LoadSpecBytesFunc, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	loadSpec, exists := r.reg[gvk]
	if !exists {
		return nil, ErrKindNotRegistered
	}
	return loadSpec, nil
}

type Loader struct {
	Registry LoaderRegistry
	Template *template.Template
}

func (l *Loader) LoadSpecBytes(specFileData []byte, workingDir string, templateData map[string]any, imageOverrides map[string]string) (Spec, error) {
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
	return loadSpecFunc(specDataBuf.Bytes(), workingDir, imageOverrides)
}

func (l *Loader) LoadSpecFile(path string, templateData map[string]any, imageOverrides map[string]string) (Spec, error) {
	specFileData, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	return l.LoadSpecBytes(specFileData, filepath.Dir(path), templateData, imageOverrides)
}

var DefaultLoader = &Loader{Registry: NewLoaderRegistry(), Template: template.New("").Option("missingkey=error")}
var registerKind = DefaultLoader.Registry.RegisterKind
