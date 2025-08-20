package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"iter"
	"maps"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing/fstest"

	"github.com/blang/semver/v4"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"

	"github.com/joelanford/kpm/internal/pkg/bundle/registry/internal"
)

type metadata struct {
	annotationsFile  AnnotationsFile
	propertiesFile   *PropertiesFile
	dependenciesFile *DependenciesFile
}

func (m *metadata) PackageName() string {
	return m.annotationsFile.Value().Annotations[annotationPackage]
}

func (m *metadata) Annotations() AnnotationsFile {
	return m.annotationsFile
}

func (m *metadata) Properties() *PropertiesFile {
	return m.propertiesFile
}

func (m *metadata) Dependencies() *DependenciesFile {
	return m.dependenciesFile
}

func (m *metadata) All() iter.Seq[File[any]] {
	return func(yield func(File[any]) bool) {
		if !yield(toAnyFile(m.annotationsFile)) {
			return
		}
		if m.propertiesFile != nil && !yield(toAnyFile(*m.propertiesFile)) {
			return
		}
		if m.dependenciesFile != nil && !yield(toAnyFile(*m.dependenciesFile)) {
			return
		}
	}
}

func toAnyFile[T any](in File[T]) File[any] {
	return NewPrecomputedFile[any](in.Name(), in.Data(), in.Value())
}

func (m *metadata) addToFS(fsys fstest.MapFS) {
	for f := range m.All() {
		path := filepath.Join(metadataDirectory, f.Name())
		fsys[path] = &fstest.MapFile{Data: f.Data()}
	}
}

type AnnotationsFile = File[Annotations]
type PropertiesFile = File[Properties]
type DependenciesFile = File[Dependencies]

type Annotations struct {
	Annotations map[string]string `json:"annotations"`
}

type Properties struct {
	Properties []Property `json:"properties"`
}

type Dependencies struct {
	Dependencies []Dependency `json:"dependencies"`
}

type Property struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

type Dependency = Property

type MetadataLoader interface {
	Load() (*metadata, error)
}
type metadataFSLoader struct {
	fsys fs.FS
}

func (m *metadataFSLoader) loadMetadata() (*metadata, error) {
	a, aErr := m.loadAnnotations()
	p, pErr := m.loadProperties()
	d, dErr := m.loadDependencies()
	if err := errors.Join(aErr, pErr, dErr); err != nil {
		return nil, err
	}
	return &metadata{
		annotationsFile:  *a,
		propertiesFile:   p,
		dependenciesFile: d,
	}, nil
}

func (m *metadataFSLoader) Load() (*metadata, error) {
	metadata, err := m.loadMetadata()
	if err != nil {
		return nil, err
	}
	if err := metadata.validate(); err != nil {
		return nil, err
	}
	return metadata, nil
}

const (
	annotationsFileName  = "annotations.yaml"
	propertiesFileName   = "properties.yaml"
	dependenciesFileName = "dependencies.yaml"
)

func (m *metadataFSLoader) loadAnnotations() (*AnnotationsFile, error) {
	data, err := fs.ReadFile(m.fsys, annotationsFileName)
	if err != nil {
		return nil, err
	}
	f, err := NewYAMLDataFile[Annotations](annotationsFileName, data)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %v", annotationsFileName, err)
	}
	return f, err
}

func (m *metadataFSLoader) loadProperties() (*PropertiesFile, error) {
	data, err := fs.ReadFile(m.fsys, propertiesFileName)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	f, err := NewYAMLDataFile[Properties](propertiesFileName, data)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %v", propertiesFileName, err)
	}
	return f, err
}
func (m *metadataFSLoader) loadDependencies() (*DependenciesFile, error) {
	data, err := fs.ReadFile(m.fsys, dependenciesFileName)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	f, err := NewYAMLDataFile[Dependencies](dependenciesFileName, data)
	if err != nil {
		return nil, fmt.Errorf("error parsing %s: %v", dependenciesFileName, err)
	}
	return f, err
}

func (m *metadata) validate() error {
	validations := []func() error{
		m.validateAnnotations,
		m.validateProperties,
		m.validateDependencies,
	}
	var validationErrors []error
	for _, fn := range validations {
		if err := fn(); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}
	return errors.Join(validationErrors...)
}

const (
	annotationMediaType = "operators.operatorframework.io.bundle.mediatype.v1"
	annotationManifests = "operators.operatorframework.io.bundle.manifests.v1"
	annotationMetadata  = "operators.operatorframework.io.bundle.metadata.v1"
	annotationPackage   = "operators.operatorframework.io.bundle.package.v1"
)

func (m *metadata) validateAnnotations() error {
	if err := func() error {
		if len(m.annotationsFile.Value().Annotations) == 0 {
			return errors.New("no annotations found")
		}

		requireValid := func(validateFn func(string) error) func(string, string) error {
			return func(key, value string) error {
				if err := validateFn(value); err != nil {
					return fmt.Errorf("invalid value for annotation key %q: %v", key, err)
				}
				return nil
			}
		}
		requireEqual := func(expectedValue string) func(string, string) error {
			return requireValid(func(actualValue string) error {
				if actualValue != expectedValue {
					return fmt.Errorf("requires value %q, but found %q", expectedValue, actualValue)
				}
				return nil
			})
		}

		requiredAnnotations := map[string]func(k, v string) error{
			annotationMediaType: requireEqual(mediaType),
			annotationManifests: requireEqual(manifestsDirectory),
			annotationMetadata:  requireEqual(metadataDirectory),
			annotationPackage: requireValid(func(in string) error {
				errs := validation.IsDNS1123Subdomain(in)
				if len(errs) > 0 {
					return errors.New(strings.Join(errs, ", "))
				}
				return nil
			}),
		}

		var validationErrors []error
		for _, key := range slices.Sorted(maps.Keys(requiredAnnotations)) {
			value, ok := m.annotationsFile.Value().Annotations[key]
			if !ok {
				validationErrors = append(validationErrors, fmt.Errorf("required key %q not found", key))
				continue
			}

			validateFn := requiredAnnotations[key]
			if err := validateFn(key, value); err != nil {
				validationErrors = append(validationErrors, err)
				continue
			}
		}
		return errors.Join(validationErrors...)
	}(); err != nil {
		return fmt.Errorf("invalid annotations: %v", err)
	}
	return nil
}

func (m *metadata) validateProperties() error {
	if m.propertiesFile == nil {
		return nil
	}
	if err := internal.DoAll(
		m.validatePropertyTypeValues,
		m.validatePropertiesNoReservedUsage,
	); err != nil {
		return fmt.Errorf("invalid properties: %v", err)
	}
	return nil
}

func (m *metadata) validatePropertyTypeValues() error {
	var errs []error
	for i, prop := range m.propertiesFile.Value().Properties {
		validator := validatorFor(prop.Type, propertyScheme, true)
		if err := validator(prop.Value); err != nil {
			errs = append(errs, fmt.Errorf("property at index %d with type %q is invalid: %w", i, prop.Type, err))
			continue
		}
	}
	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("invalid values: %v", err)
	}
	return nil
}

func (m *metadata) validatePropertiesNoReservedUsage() error {
	reserved := sets.New[string](
		typePropertyPackage,
		typePropertyGVK,
	)

	found := sets.New[string]()
	for _, prop := range m.propertiesFile.Value().Properties {
		if reserved.Has(prop.Type) {
			found.Insert(prop.Type)
		}
	}
	if found.Len() > 0 {
		return fmt.Errorf("found reserved properties %v: these properties are reserved for use by OLM", sets.List(found))
	}
	return nil
}

func (m *metadata) validateDependencies() error {
	if m.dependenciesFile == nil {
		return nil
	}
	var errs []error
	for i, dep := range m.dependenciesFile.Value().Dependencies {
		validator := validatorFor(dep.Type, dependencyScheme, false)
		if err := validator(dep.Value); err != nil {
			errs = append(errs, fmt.Errorf("dependency at index %d with type %q is invalid: %w", i, dep.Type, err))
			continue
		}
	}

	if err := errors.Join(errs...); err != nil {
		return fmt.Errorf("invalid dependencies: %v", err)
	}
	return nil
}

type typeValidator interface {
	validate() error
}

func validatorFor(typ string, scheme map[string]func() typeValidator, allowAny bool) func(json.RawMessage) error {
	return func(value json.RawMessage) error {
		var errs []error

		fn, ok := scheme[typ]
		if !ok {
			fn = func() typeValidator { return &anyJSON{} }
		}
		v := fn()

		// validate type
		if len(typ) == 0 {
			errs = append(errs, fmt.Errorf("type is required"))
		} else if !ok && !allowAny {
			errs = append(errs, fmt.Errorf("unknown type %q", typ))
		}

		// validate value
		if len(value) == 0 {
			errs = append(errs, fmt.Errorf("value is required"))
		} else if err := json.Unmarshal(value, v); err != nil {
			errs = append(errs, fmt.Errorf("failed to unmarshal value %q: %v", string(value), err))
		} else if err := v.validate(); err != nil {
			errs = append(errs, fmt.Errorf("failed to validate value %q: %v", string(value), err))
		}

		return errors.Join(errs...)
	}
}

const (
	typePropertyPackage         = "olm.package"
	typePropertyGVK             = "olm.gvk"
	typePropertyPackageRequired = "olm.package.required"
	typePropertyGVKRequired     = "olm.gvk.required"

	typeDependencyPackage = "olm.package"
	typeDependencyGVK     = "olm.gvk"
)

var (
	propertyScheme = map[string]func() typeValidator{
		typePropertyPackageRequired: func() typeValidator { return &propertyPackageRequired{} },
		typePropertyGVKRequired:     func() typeValidator { return &gvk{} },
	}

	dependencyScheme = map[string]func() typeValidator{
		typeDependencyPackage: func() typeValidator { return &dependencyPackage{} },
		typeDependencyGVK:     func() typeValidator { return &gvk{} },
	}
)

type propertyPackageRequired struct {
	PackageName  string `json:"packageName"`
	VersionRange string `json:"versionRange"`
}

func (p *propertyPackageRequired) validate() error {
	var errs []error
	if err := validatePackageName(p.PackageName); err != nil {
		errs = append(errs, err)
	}
	if err := validateVersionRange("versionRange", p.VersionRange); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

type anyJSON struct {
	json.RawMessage
}

func (p *anyJSON) validate() error {
	return nil
}

type dependencyPackage struct {
	PackageName string `json:"packageName"`
	Version     string `json:"version"`
}

func (d *dependencyPackage) validate() error {
	var errs []error
	if err := validatePackageName(d.PackageName); err != nil {
		errs = append(errs, err)
	}
	if err := validateVersionRange("version", d.Version); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

type gvk struct {
	Group   string `json:"group"`
	Version string `json:"version"`
	Kind    string `json:"kind"`
}

const (
	versionPattern = `^v[0-9]+((alpha|beta)[0-9]+)?$`
	kindPattern    = `^[A-Z][A-Za-z0-9]*$`
)

var (
	versionRegexp = regexp.MustCompile(versionPattern)
	kindRegexp    = regexp.MustCompile(kindPattern)
)

func (p *gvk) validate() error {
	var errs []error
	if p.Group == "" {
		errs = append(errs, fmt.Errorf("group is required"))
	} else if problems := validation.IsDNS1123Subdomain(p.Group); len(problems) > 0 {
		errs = append(errs, fmt.Errorf("group %q is invalid: %v", p.Group, strings.Join(problems, ", ")))
	}
	if p.Version == "" {
		errs = append(errs, fmt.Errorf("version is required"))
	} else if !versionRegexp.MatchString(p.Version) {
		errs = append(errs, fmt.Errorf("version %q is invalid: must match pattern %s", p.Version, versionPattern))
	}
	if p.Kind == "" {
		errs = append(errs, fmt.Errorf("kind is required"))
	} else if !kindRegexp.MatchString(p.Kind) {
		errs = append(errs, fmt.Errorf("kind %q is invalid: must match pattern %s", p.Kind, kindPattern))
	}
	return errors.Join(errs...)
}

func validatePackageName(name string) error {
	if name == "" {
		return errors.New("packageName is required")
	}
	if problems := validation.IsDNS1123Subdomain(name); len(problems) > 0 {
		return fmt.Errorf("packageName %q is invalid: %v", name, strings.Join(problems, ", "))
	}
	return nil
}

func validateVersionRange(fieldName, versionRange string) error {
	if versionRange == "" {
		return fmt.Errorf("%s is required", fieldName)
	}
	if _, err := semver.ParseRange(versionRange); err != nil {
		return fmt.Errorf("%s %q is invalid: %v", fieldName, versionRange, err)
	}
	return nil
}
