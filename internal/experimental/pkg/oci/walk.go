package oci

import (
	"context"
	_ "crypto/sha256"
	"errors"
	"fmt"
	"io"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/exp/slices"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/registry"
)

type (
	WalkFunc func(context.Context, []ocispec.Descriptor, ocispec.Descriptor, error) error
	Walker   interface {
		Reference(ctx context.Context, ref string, walkFn WalkFunc) error
		Descriptor(ctx context.Context, desc ocispec.Descriptor, walkFn WalkFunc) error
	}
)

var (
	ErrSkip = errors.New("OCI subtree skipped")

	defaultSuccessorsFunc = content.Successors
	defaultReferrersFunc  = func(ctx context.Context, t oras.ReadOnlyGraphTarget, desc ocispec.Descriptor) ([]ocispec.Descriptor, error) {
		return registry.Referrers(ctx, t, desc, "")
	}
)

type (
	WalkOption     func(*walker)
	SuccessorsFunc func(ctx context.Context, f content.Fetcher, desc ocispec.Descriptor) ([]ocispec.Descriptor, error)
	ReferrersFunc  func(ctx context.Context, target oras.ReadOnlyGraphTarget, desc ocispec.Descriptor) ([]ocispec.Descriptor, error)
)

func New(t oras.ReadOnlyGraphTarget, opts ...WalkOption) Walker {
	w := &walker{
		store:          t,
		successorsFunc: defaultSuccessorsFunc,
		referrersFunc:  defaultReferrersFunc,
	}
	for _, opt := range opts {
		opt(w)
	}
	return w
}

func WithSuccessorsFunc(f SuccessorsFunc) WalkOption {
	return func(w *walker) {
		w.successorsFunc = f
	}
}
func WithReferrersFunc(f ReferrersFunc) WalkOption {
	return func(w *walker) {
		w.referrersFunc = f
	}
}

type walker struct {
	store          oras.ReadOnlyGraphTarget
	successorsFunc SuccessorsFunc
	referrersFunc  ReferrersFunc
}

func (w *walker) Reference(ctx context.Context, ref string, walkFn WalkFunc) error {
	desc, err := w.store.Resolve(ctx, ref)
	if err != nil {
		return fmt.Errorf("failed to resolve %q: %w", ref, err)
	}
	return w.Descriptor(ctx, desc, walkFn)
}

func (w *walker) Descriptor(ctx context.Context, desc ocispec.Descriptor, walkFn WalkFunc) error {
	return w.walk(ctx, nil, desc, true, walkFn)
}

func (w *walker) walk(ctx context.Context, path []ocispec.Descriptor, descriptor ocispec.Descriptor, recurse bool, walkFn WalkFunc) error {

	var (
		successors []ocispec.Descriptor
		referrers  []ocispec.Descriptor
		err        error
	)
	var errs []error
	if isManifest(descriptor) {
		successors, err = w.successorsFunc(ctx, w.store, descriptor)
		if err != nil {
			errs = append(errs, err)
		}
		referrers, err = w.referrersFunc(ctx, w.store, descriptor)
		if err != nil {
			errs = append(errs, err)
		}
	}

	if err := walkFn(ctx, path, descriptor, errors.Join(errs...)); err != nil {
		if errors.Is(err, ErrSkip) {
			return nil
		}
		return err
	}

	if (len(successors) == 0 && len(referrers) == 0) || !recurse {
		return nil
	}

	path = append(slices.Clone(path), descriptor)
	if err := w.walkDescriptors(ctx, path, referrers, false, walkFn); err != nil {
		return err
	}
	if err := w.walkDescriptors(ctx, path, successors, true, walkFn); err != nil {
		return err
	}
	return nil
}

func (w *walker) walkDescriptors(ctx context.Context, path, descriptors []ocispec.Descriptor, recurse bool, walkFn WalkFunc) error {
	for _, descriptor := range descriptors {
		if err := w.walk(ctx, path, descriptor, recurse, walkFn); err != nil {
			return err
		}
	}
	return nil
}

func isManifest(descriptor ocispec.Descriptor) bool {
	f := &isManifestFetcher{}
	_, _ = content.Successors(context.Background(), f, descriptor)
	return f.called
}

type isManifestFetcher struct {
	called bool
}

func (f *isManifestFetcher) Fetch(_ context.Context, _ ocispec.Descriptor) (io.ReadCloser, error) {
	f.called = true
	return nil, errors.New("done")
}
