package v1

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/util/validation"
	"sigs.k8s.io/yaml"

	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/operator-framework/operator-registry/pkg/registry"
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
	fsys         fs.FS
	annotations  Annotations
	properties   Properties
	dependencies Dependencies
}

type Annotations struct {
	Annotations map[string]string `json:"annotations"`
}

type Property struct {
	Type  string          `json:"type"`
	Value json.RawMessage `json:"value"`
}
type Properties struct {
	Properties []Property `json:"properties"`
}

type Dependencies struct {
	Dependencies []Property `json:"dependencies"`
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
	if err := yaml.Unmarshal(annotationsData, &m.annotations, yaml.DisallowUnknownFields); err != nil {
		return fmt.Errorf("failed to extract annotations from %q: %w", AnnotationsFile, err)
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
	if err := yaml.Unmarshal(propertiesData, &m.properties, yaml.DisallowUnknownFields); err != nil {
		return fmt.Errorf("failed to extract properties from %q: %w", PropertiesFile, err)
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
	if err := yaml.Unmarshal(dependenciesData, &m.dependencies, yaml.DisallowUnknownFields); err != nil {
		return fmt.Errorf("failed to extract dependencies from %q: %w", DependenciesFile, err)
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
		if m.annotations.Annotations == nil {
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
			value := m.annotations.Annotations[key]
			validateFn, ok := requiredAnnotations[key]
			if !ok {
				validationErrors = append(validationErrors, fmt.Errorf("required key %q not found", key))
				continue
			}

			if err := validateFn(key, value); err != nil {
				validationErrors = append(validationErrors, err)
				continue
			}
		}
		return errors.Join(validationErrors...)
	}(); err != nil {
		return fmt.Errorf("invalid metadata annotations: %v", err)
	}
	return nil
}

func (m *metadata) validateProperties() error {
	// Once properties are parsed successfully, there are no further
	// aspects to validate.
	return nil
}

func (m *metadata) validateDependencies() error {
	if err := func() error {
		if m.dependencies.Dependencies == nil {
			return nil
		}

		unmarshalInto := func(into any) func(json.RawMessage) error {
			return func(jsonVal json.RawMessage) error {
				return yaml.Unmarshal(jsonVal, into)
			}
		}

		dependencyTypeValidators := map[string]func(json.RawMessage) error{
			"olm.gvk":        unmarshalInto(&property.GVKRequired{}),
			"olm.package":    unmarshalInto(&property.PackageRequired{}),
			"olm.constraint": unmarshalInto(&registry.CelConstraint{}),
		}

		var validationErrors []error
		for _, dependency := range m.dependencies.Dependencies {
			typeValidator, ok := dependencyTypeValidators[dependency.Type]
			if !ok {
				validationErrors = append(validationErrors, fmt.Errorf("dependency type %q not recognized", dependency.Type))
				continue
			}
			if err := typeValidator(dependency.Value); err != nil {
				validationErrors = append(validationErrors, fmt.Errorf("failed to parse value for dependency %q: %v", dependency.Type, err))
				continue
			}
		}
		return errors.Join(validationErrors...)
	}(); err != nil {
		return fmt.Errorf("invalid metadata dependencies: %v", err)
	}
	return nil
}
