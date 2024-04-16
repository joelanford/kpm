package transport

import (
	"context"
	"errors"
	"fmt"
	"github.com/joelanford/kpm/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"strings"
	"sync"
)

type Transport interface {
	ParseReference(string) (Target, error)
	Protocol() string
}

type Target interface {
	Push(context.Context, oci.Artifact) (string, ocispec.Descriptor, error)
	String() string
}

var all = transport{transports: make(map[string]Transport)}

func TargetFor(ref string) (Target, error) {
	return all.TargetFor(ref)
}

type transport struct {
	mu         sync.RWMutex
	transports map[string]Transport
}

func (t *transport) Register(transport Transport) error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if _, ok := all.transports[transport.Protocol()]; ok {
		return fmt.Errorf("transport %q already registered", transport.Protocol())
	}
	t.transports[transport.Protocol()] = transport
	return nil
}

func (t *transport) TargetFor(ref string) (Target, error) {
	protocol, ref, ok := strings.Cut(ref, ":")
	if !ok {
		return nil, errors.New("invalid reference: no protocol specified")
	}

	t.mu.RLock()
	defer t.mu.RUnlock()
	tport, ok := all.transports[protocol]
	if !ok {
		return nil, fmt.Errorf("transport %q not registered", protocol)
	}
	return tport.ParseReference(ref)
}
