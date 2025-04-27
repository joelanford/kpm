package loader

import (
	"fmt"
	"sync"

	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/joelanford/kpm/internal/builder"
)

var (
	ErrKindNotRegistered     = fmt.Errorf("kind is not registered")
	ErrKindAlreadyRegistered = fmt.Errorf("kind is already registered")

	DefaultRegistry = NewLoaderRegistry()
)

type LoaderRegistry struct {
	reg map[schema.GroupVersionKind]LoadSpecBytesFunc
	mu  sync.RWMutex
}

func NewLoaderRegistry() *LoaderRegistry {
	return &LoaderRegistry{reg: make(map[schema.GroupVersionKind]LoadSpecBytesFunc)}
}

type LoadSpecBytesFunc func([]byte, string) (builder.Builder, error)

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
