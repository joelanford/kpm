package v1

import (
	"errors"
	"fmt"
	"io/fs"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
)

const MediaType = "registry+v1"

type Bundle struct {
	fsys      fs.FS
	manifests manifests
	metadata  metadata

	csv v1alpha1.ClusterServiceVersion
}

func LoadFS(fsys fs.FS) (*Bundle, error) {
	metadataFsys, err := fs.Sub(fsys, "metadata")
	if err != nil {
		return nil, err
	}
	manifestsFsys, err := fs.Sub(fsys, "manifests")
	if err != nil {
		return nil, err
	}
	b := &Bundle{
		fsys:      fsys,
		metadata:  metadata{fsys: metadataFsys},
		manifests: manifests{fsys: manifestsFsys},
	}
	for _, fn := range []func() error{
		b.load,
		b.validate,
		b.complete,
	} {
		if err := fn(); err != nil {
			return nil, err
		}
	}
	return b, nil
}

func (b *Bundle) load() error {
	if err := do(
		b.metadata.load,
		b.manifests.load,
	); err != nil {
		return fmt.Errorf("failed to load bundle: %v", err)
	}
	return nil
}

func (b *Bundle) validate() error {
	if err := do(
		b.metadata.validate,
		b.manifests.validate,
	); err != nil {
		return fmt.Errorf("failed to validate bundle: %v", err)
	}
	return nil
}

func (b *Bundle) complete() error {
	if err := do(
		b.extractCSV,
	); err != nil {
		return fmt.Errorf("failed to complete bundle: %v", err)
	}
	return nil
}

func (b *Bundle) extractCSV() error {
	for _, mf := range b.manifests.manifestFiles {
		for _, obj := range mf.objects {
			if obj.GetObjectKind().GroupVersionKind().Kind != v1alpha1.ClusterServiceVersionKind {
				continue
			}
			csv := obj.(*v1alpha1.ClusterServiceVersion)
			b.csv = *csv
			return nil
		}
	}
	// this should never happen because the earlier validate step ensures there is exactly one CSV in the manifests.
	return fmt.Errorf("failed to find ClusterServiceVersion in bundle")
}

func do(funcs ...func() error) error {
	var errs []error
	for _, fn := range funcs {
		errs = append(errs, fn())
	}
	return errors.Join(errs...)
}
