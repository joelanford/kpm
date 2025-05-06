package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"regexp"
	"slices"
	"strings"

	"github.com/blang/semver/v4"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"
)

const (
	MetadataDirectory = "metadata/"

	AnnotationsFile  = "annotations.yaml"
	PropertiesFile   = "properties.yaml"
	DependenciesFile = "dependencies.yaml"

	AnnotationMediaType = "operators.operatorframework.io.bundle.mediatype.v1"
	AnnotationManifests = "operators.operatorframework.io.bundle.manifests.v1"
	AnnotationMetadata  = "operators.operatorframework.io.bundle.metadata.v1"
	AnnotationPackage   = "operators.operatorframework.io.bundle.package.v1"
)

type metadata struct {
	fsys             fs.FS
	annotationsFile  Annotations
	propertiesFile   Properties
	dependenciesFile Dependencies
}

type Annotations struct {
	Annotations map[string]string `json:"annotations"`
}

type Properties struct {
	Properties []Property `json:"properties"`
}

type Dependencies struct {
	Dependencies []Dependency `json:"dependencies"`
}

func (m *metadata) load() error {
	var loadErrs []error
	for _, loadFn := range []func() error{
		m.loadMetadataAnnotations,
		m.loadMetadataProperties,
		m.loadMetadataDependencies,
	} {
		if err := loadFn(); err != nil {
			loadErrs = append(loadErrs, err)
		}
	}
	return errors.Join(loadErrs...)
}

func (m *metadata) loadMetadataAnnotations() error {
	annotationsData, err := fs.ReadFile(m.fsys, AnnotationsFile)
	if err != nil {
		return err
	}
	if err := yaml.Unmarshal(annotationsData, &m.annotationsFile, yaml.DisallowUnknownFields); err != nil {
		return fmt.Errorf("failed to load annotations from %q: %w", AnnotationsFile, err)
	}
	return nil
}

func (m *metadata) loadMetadataProperties() error {
	propertiesData, err := fs.ReadFile(m.fsys, PropertiesFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := yaml.Unmarshal(propertiesData, &m.propertiesFile, yaml.DisallowUnknownFields); err != nil {
		return fmt.Errorf("failed to load properties from %q: %w", PropertiesFile, err)
	}
	return nil
}

func (m *metadata) loadMetadataDependencies() error {
	dependenciesData, err := fs.ReadFile(m.fsys, DependenciesFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if err := yaml.Unmarshal(dependenciesData, &m.dependenciesFile, yaml.DisallowUnknownFields); err != nil {
		return fmt.Errorf("failed to load dependencies from %q: %w", DependenciesFile, err)
	}
	return nil
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

func (m *metadata) validateAnnotations() error {
	if err := func() error {
		if m.annotationsFile.Annotations == nil {
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
			AnnotationMediaType: requireEqual(MediaType),
			AnnotationManifests: requireEqual(ManifestsDirectory),
			AnnotationMetadata:  requireEqual(MetadataDirectory),
			AnnotationPackage: requireValid(func(in string) error {
				errs := validation.IsDNS1123Subdomain(in)
				if len(errs) > 0 {
					return errors.New(strings.Join(errs, ", "))
				}
				return nil
			}),
		}

		var validationErrors []error
		for _, key := range slices.Sorted(maps.Keys(requiredAnnotations)) {
			value, ok := m.annotationsFile.Annotations[key]
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
	if err := do(
		m.validatePropertyTypeValues,
		m.validatePropertiesNoReservedUsage,
	); err != nil {
		return fmt.Errorf("invalid properties: %v", err)
	}
	return nil
}

func (m *metadata) validatePropertyTypeValues() error {
	var errs []error
	for i, prop := range m.propertiesFile.Properties {
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
		TypePropertyPackage,
		TypePropertyGVK,
	)

	found := sets.New[string]()
	for _, prop := range m.propertiesFile.Properties {
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
	var errs []error
	for i, dep := range m.dependenciesFile.Dependencies {
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
	Validate() error
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
		} else if err := v.Validate(); err != nil {
			errs = append(errs, fmt.Errorf("failed to validate value %q: %v", string(value), err))
		}

		return errors.Join(errs...)
	}
}

const (
	TypePropertyPackage         = "olm.package"
	TypePropertyGVK             = "olm.gvk"
	TypePropertyPackageRequired = "olm.package.required"
	TypePropertyGVKRequired     = "olm.gvk.required"

	TypeDependencyPackage = "olm.package"
	TypeDependencyGVK     = "olm.gvk"
)

var (
	propertyScheme = map[string]func() typeValidator{
		TypePropertyPackageRequired: func() typeValidator { return &PropertyPackageRequired{} },
		TypePropertyGVKRequired:     func() typeValidator { return &GVK{} },
	}

	dependencyScheme = map[string]func() typeValidator{
		TypeDependencyPackage: func() typeValidator { return &DependencyPackage{} },
		TypeDependencyGVK:     func() typeValidator { return &GVK{} },
	}
)

type Property struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}

type Dependency = Property

type PropertyPackageRequired struct {
	PackageName  string `json:"packageName"`
	VersionRange string `json:"versionRange"`
}

func (p *PropertyPackageRequired) Validate() error {
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

func (p *anyJSON) Validate() error {
	return nil
}

type DependencyPackage struct {
	PackageName string `json:"packageName"`
	Version     string `json:"version"`
}

func (d *DependencyPackage) Validate() error {
	var errs []error
	if err := validatePackageName(d.PackageName); err != nil {
		errs = append(errs, err)
	}
	if err := validateVersionRange("version", d.Version); err != nil {
		errs = append(errs, err)
	}
	return errors.Join(errs...)
}

type GVK struct {
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

func (p *GVK) Validate() error {
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
