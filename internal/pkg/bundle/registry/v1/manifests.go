package v1

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"iter"
	"maps"
	"path/filepath"
	"slices"
	"strings"
	"testing/fstest"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"

	"github.com/joelanford/kpm/internal/pkg/bundle/registry/internal"
)

type manifests struct {
	csv    File[*v1alpha1.ClusterServiceVersion]
	crds   []File[*apiextensionsv1.CustomResourceDefinition]
	others []File[client.Object]
}

func (m *manifests) CSV() File[*v1alpha1.ClusterServiceVersion] {
	return m.csv
}

func (m *manifests) CRDs() []File[*apiextensionsv1.CustomResourceDefinition] {
	return m.crds
}

func (m *manifests) Others() []File[client.Object] {
	return m.others
}

func (m *manifests) All() iter.Seq[File[client.Object]] {
	return func(yield func(File[client.Object]) bool) {
		if !yield(toObjectFile(m.csv)) {
			return
		}
		for _, crd := range m.crds {
			if !yield(toObjectFile(crd)) {
				return
			}
		}
		for _, other := range m.others {
			if !yield(other) {
				return
			}
		}
	}
}

func toObjectFile[T client.Object](in File[T]) File[client.Object] {
	return NewPrecomputedFile[client.Object](in.Name(), in.Data(), in.Value())
}

func (m *manifests) addToFS(fsys fstest.MapFS) {
	for f := range m.All() {
		path := filepath.Join(manifestsDirectory, f.Name())
		fsys[path] = &fstest.MapFile{Data: f.Data()}
	}
}

type ManifestsLoader interface {
	Load() (*manifests, error)
}

type manifestsFSLoader struct {
	fsys fs.FS
}

func (m *manifestsFSLoader) Load() (*manifests, error) {
	files, err := m.loadFiles()
	if err != nil {
		return nil, err
	}
	return files.toManifests()
}

func (m *manifestsFSLoader) loadFiles() (manifestFiles, error) {
	var (
		files    manifestFiles
		loadErrs []error
	)

	if err := fs.WalkDir(m.fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			loadErrs = append(loadErrs, err)
			return nil
		}
		if d.IsDir() {
			return nil
		}

		f, err := m.fsys.Open(path)
		if err != nil {
			loadErrs = append(loadErrs, err)
			return nil
		}
		defer f.Close()

		mf, err := newManifestFileFromReader(f, path)
		if err != nil {
			loadErrs = append(loadErrs, err)
			return nil
		}
		files = append(files, *mf)
		return nil
	}); err != nil {
		panic("all errors should be collected by the WalkDirFunc")
	}
	if err := errors.Join(loadErrs...); err != nil {
		return nil, fmt.Errorf("failed to load manifests: %v", err)
	}
	return files, nil
}

type manifestFiles []File[[]client.Object]

func (m manifestFiles) toManifests() (*manifests, error) {
	if err := m.validate(); err != nil {
		return nil, err
	}
	var manifests manifests
	for _, mf := range m {
		if len(mf.Value()) != 1 {
			panic("validation should have ensured that each manifest file has exactly one object")
		}
		obj := mf.Value()[0]
		switch obj.GetObjectKind().GroupVersionKind().Kind {
		case v1alpha1.ClusterServiceVersionKind:
			csvObj := obj.(*v1alpha1.ClusterServiceVersion)
			manifests.csv = NewPrecomputedFile(mf.Name(), mf.Data(), csvObj)
		case "CustomResourceDefinition":
			crdObj := obj.(*apiextensionsv1.CustomResourceDefinition)
			manifests.crds = append(manifests.crds, NewPrecomputedFile(mf.Name(), mf.Data(), crdObj))
		default:
			manifests.others = append(manifests.others, NewPrecomputedFile(mf.Name(), mf.Data(), obj))
		}
	}
	return &manifests, nil
}

func (m manifestFiles) validate() error {
	var validationErrors []error
	for _, validationFn := range []func() error{
		m.validateNoSubDirectories,
		m.validateOneObjectPerFile,
		m.validateExactlyOneCSV,
		m.validateSupportedKinds,
	} {
		if err := validationFn(); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}
	if err := errors.Join(validationErrors...); err != nil {
		return fmt.Errorf("invalid registry+v1 manifests: %v", err)
	}
	return nil
}

func (m manifestFiles) validateNoSubDirectories() error {
	foundSubDirectories := map[string]struct{}{}
	for _, mf := range m {
		foundSubDirectories[filepath.Dir(mf.Name())] = struct{}{}
	}
	delete(foundSubDirectories, ".")
	if len(foundSubDirectories) == 0 {
		return nil
	}
	return fmt.Errorf("found subdirectories %v: subdirectories not allowed", slices.Sorted(maps.Keys(foundSubDirectories)))
}

func (m manifestFiles) validateOneObjectPerFile() error {
	var invalidFiles []string
	for _, mf := range m {
		if len(mf.Value()) != 1 {
			invalidFiles = append(invalidFiles, fmt.Sprintf("%q has %d", mf.Name(), len(mf.Value())))
		}
	}
	if len(invalidFiles) > 0 {
		return fmt.Errorf("manifest files must contain exactly one object: %v", strings.Join(invalidFiles, ", "))
	}
	return nil
}

func (m manifestFiles) validateExactlyOneCSV() error {
	totalCount := 0
	foundCSVs := map[string]int{}
	for _, mf := range m {
		for _, o := range mf.Value() {
			if o.GetObjectKind().GroupVersionKind().Kind == v1alpha1.ClusterServiceVersionKind {
				totalCount++
				foundCSVs[mf.Name()]++
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

func (m manifestFiles) validateSupportedKinds() error {
	var unsupported []string
	for _, mf := range m {
		fileUnsupported := sets.New[string]()
		for _, obj := range mf.Value() {
			kind := obj.GetObjectKind().GroupVersionKind().Kind
			if !internal.SupportedKinds.Has(kind) {
				fileUnsupported.Insert(kind)
			}
		}
		if len(fileUnsupported) > 0 {
			unsupported = append(unsupported, fmt.Sprintf("file %q contains %v", mf.Name(), sets.List(fileUnsupported)))
		}
	}
	if len(unsupported) > 0 {
		return fmt.Errorf("found unsupported kinds: %v", strings.Join(unsupported, ", "))
	}
	return nil
}

func newManifestFileFromReader(file io.Reader, path string) (*File[[]client.Object], error) {
	var (
		objs []client.Object
		errs []error
	)

	// We'll store the original file contents in this buffer as the
	// resource builder reads objects from the stream.
	buf := &bytes.Buffer{}
	file = io.TeeReader(file, buf)

	resource.NewLocalBuilder()
	res := resource.NewLocalBuilder().
		ContinueOnError().
		Unstructured().
		Flatten().
		Stream(file, path).
		Do()
	if err := res.Err(); err != nil {
		errs = append(errs, err)
	}
	if err := res.Visit(func(info *resource.Info, err error) error {
		if err != nil {
			errs = append(errs, err)
			return nil
		}

		u := info.Object.(*unstructured.Unstructured)
		gvk := u.GroupVersionKind()

		if internal.SupportedKindsScheme.Recognizes(gvk) {
			info.Object, _ = internal.SupportedKindsScheme.New(gvk)
			if err := runtime.DefaultUnstructuredConverter.FromUnstructured(u.Object, info.Object); err != nil {
				errs = append(errs, err)
				return nil
			}
		}

		objs = append(objs, info.Object.(client.Object))
		return nil
	}); err != nil {
		errs = append(errs, err)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	f := NewPrecomputedFile(path, buf.Bytes(), objs)
	return &f, nil
}
