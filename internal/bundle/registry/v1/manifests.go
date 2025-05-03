package v1

import (
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/cli-runtime/pkg/resource"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/operator-framework/operator-registry/pkg/lib/bundle"
)

const ManifestsDirectory = "manifests/"

type manifests struct {
	fsys          fs.FS
	manifestFiles []manifestFile
}

func (m *manifests) load() error {
	var loadErrs []error
	if err := fs.WalkDir(m.fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			loadErrs = append(loadErrs, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}
		mf, err := newManifestFile(m.fsys, path)
		if err != nil {
			loadErrs = append(loadErrs, err)
			return nil
		}
		m.manifestFiles = append(m.manifestFiles, *mf)
		return nil
	}); err != nil {
		return err
	}
	return errors.Join(loadErrs...)
}

func (m *manifests) validate() error {
	var validationErrors []error
	for _, validationFn := range []func() error{
		m.validateNoSubDirectories,
		m.validateOneManifestPerFile,
		m.validateExactlyOneCSV,
		m.validateSupportedKinds,
	} {
		if err := validationFn(); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}
	return errors.Join(validationErrors...)
}

func (m *manifests) validateNoSubDirectories() error {
	foundSubDirectories := map[string]struct{}{}
	for _, mf := range m.manifestFiles {
		foundSubDirectories[filepath.Dir(mf.filename)] = struct{}{}
	}
	delete(foundSubDirectories, ".")
	if len(foundSubDirectories) == 0 {
		return nil
	}
	return fmt.Errorf("found subdirectories %v: subdirectories not allowed", slices.Sorted(maps.Keys(foundSubDirectories)))
}

func (m *manifests) validateOneManifestPerFile() error {
	var invalidFiles []string
	for _, mf := range m.manifestFiles {
		if len(mf.objects) != 1 {
			invalidFiles = append(invalidFiles, fmt.Sprintf("%q has %d", mf.filename, len(mf.objects)))
		}
	}
	if len(invalidFiles) > 0 {
		return fmt.Errorf("found files with invalid number of objects: %v", strings.Join(invalidFiles, ", "))
	}
	return nil
}

func (m *manifests) validateExactlyOneCSV() error {
	totalCount := 0
	foundCSVs := map[string]int{}
	for _, mf := range m.manifestFiles {
		for _, o := range mf.objects {
			if o.GroupVersionKind().Kind == v1alpha1.ClusterServiceVersionKind {
				totalCount++
				foundCSVs[mf.filename]++
			}
		}
	}
	if totalCount == 0 {
		return fmt.Errorf("exactly one %s object is required, found 0", v1alpha1.ClusterServiceVersionKind)
	}
	if totalCount > 1 {
		counts := make([]string, 0, len(foundCSVs))
		for _, filename := range slices.Sorted(maps.Keys(foundCSVs)) {
			csvCount := foundCSVs[filename]
			counts = append(counts, fmt.Sprintf("%q has %d", filename, csvCount))
		}
		return fmt.Errorf("exactly one %s object is required, found %d: %v", v1alpha1.ClusterServiceVersionKind, totalCount, strings.Join(counts, ", "))
	}
	return nil
}

func (m *manifests) validateSupportedKinds() error {
	var unsupported []string
	for _, mf := range m.manifestFiles {
		for _, obj := range mf.objects {
			kind := obj.GroupVersionKind().Kind
			supported, _ := bundle.IsSupported(kind)
			if !supported {
				unsupported = append(unsupported, fmt.Sprintf("kind %q in %q", kind))
			}
		}
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("found unsupported kinds: %v", strings.Join(unsupported, ", "))
	}
	return nil
}

type manifestFile struct {
	filename string
	objects  []*unstructured.Unstructured
}

func newManifestFile(fsys fs.FS, path string) (*manifestFile, error) {
	file, err := fsys.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var (
		m    = &manifestFile{filename: path}
		errs []error
	)
	res := resource.NewLocalBuilder().Flatten().Unstructured().Stream(file, path).Do()
	if err := res.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}
		m.objects = append(m.objects, info.Object.(*unstructured.Unstructured))
		return nil
	}); err != nil {
		return nil, err
	}
	if len(errs) > 0 {
		return nil, errors.Join(errs...)
	}
	return m, nil
}
