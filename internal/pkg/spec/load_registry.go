package spec

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"
)

var (
	ErrKindNotRegistered     = fmt.Errorf("kind is not registered")
	ErrKindAlreadyRegistered = fmt.Errorf("kind is already registered")

	DefaultRegistry = NewLoaderRegistry()
)

type Registry struct {
	reg map[schema.GroupVersionKind]LoadSpecFunc
	mu  sync.RWMutex
}

func NewLoaderRegistry() *Registry {
	return &Registry{reg: make(map[schema.GroupVersionKind]LoadSpecFunc)}
}

type LoadSpecFunc func([]byte, string) (Spec, error)

func (r *Registry) RegisterKind(gvk schema.GroupVersionKind, loadSpecFunc LoadSpecFunc) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.reg[gvk]; exists {
		return ErrKindAlreadyRegistered
	}
	r.reg[gvk] = loadSpecFunc
	return nil
}

func (r *Registry) GetLoadSpecFunc(gvk schema.GroupVersionKind) (LoadSpecFunc, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	loadSpec, exists := r.reg[gvk]
	if !exists {
		return nil, ErrKindNotRegistered
	}
	return loadSpec, nil
}
