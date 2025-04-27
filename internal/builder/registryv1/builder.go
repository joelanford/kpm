package registryv1

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation"
	"k8s.io/cli-runtime/pkg/resource"
	"sigs.k8s.io/yaml"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
	"github.com/operator-framework/operator-registry/alpha/property"
	"github.com/operator-framework/operator-registry/pkg/registry"

	specsv1 "github.com/joelanford/kpm/internal/api/specs/v1"
	"github.com/joelanford/kpm/internal/builder"
	"github.com/joelanford/kpm/internal/loader"
)

type registryV1Builder struct {
	release          string
	extraLabels      map[string]string
	extraAnnotations map[string]string
	root             fs.FS

	csv                 *v1alpha1.ClusterServiceVersion
	manifests           map[string][]*unstructured.Unstructured
	metadataAnnotations map[string]string
	properties          []property.Property
	dependencies        []property.Property
}

func (b *registryV1Builder) Build(_ context.Context) (*builder.ID, builder.Manifest, error) {
	if err := b.complete(); err != nil {
		return nil, nil, err
	}
	if err := b.validate(); err != nil {
		return nil, nil, err
	}

	labels := make(map[string]string, len(b.extraLabels)+len(b.metadataAnnotations))
	maps.Copy(labels, b.extraLabels)
	maps.Copy(labels, b.metadataAnnotations)

	rel, err := builder.NewRelease(b.release)
	if err != nil {
		return nil, nil, err
	}
	id := &builder.ID{
		Name:    b.metadataAnnotations[annotationPackage],
		Version: b.csv.Spec.Version.Version,
		Release: *rel,
	}
	return id, &RegistryV1Writer{
		labels:      labels,
		annotations: b.extraAnnotations,
		root:        b.root,
	}, nil
}

func (b *registryV1Builder) complete() error {
	var extractErrs []error
	for _, extractFn := range []func() error{
		b.extractMetadataAnnotations,
		b.extractMetadataProperties,
		b.extractMetadataDependencies,
		b.extractManifests,
	} {
		if err := extractFn(); err != nil {
			extractErrs = append(extractErrs, err)
		}
	}
	if len(extractErrs) > 0 {
		return errors.Join(extractErrs...)
	}

	if b.release == "" {
		b.release = "0"
	}
	return errors.Join(extractErrs...)
}

func (b *registryV1Builder) extractMetadataAnnotations() error {
	annotationsData, err := fs.ReadFile(b.root, annotationsFile)
	if err != nil {
		return err
	}
	var annotations struct {
		Annotations map[string]string `json:"annotations"`
	}
	if err := yaml.Unmarshal(annotationsData, &annotations, yaml.DisallowUnknownFields); err != nil {
		return fmt.Errorf("failed to extract annotations from %q: %w", annotationsFile, err)
	}
	b.metadataAnnotations = annotations.Annotations
	return nil
}

func (b *registryV1Builder) extractMetadataProperties() error {
	propertiesData, err := fs.ReadFile(b.root, propertiesFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	var properties struct {
		Properties []property.Property `json:"properties"`
	}
	if err := yaml.Unmarshal(propertiesData, &properties, yaml.DisallowUnknownFields); err != nil {
		return fmt.Errorf("failed to extract properties from %q: %w", propertiesFile, err)
	}
	b.properties = properties.Properties
	return nil
}

func (b *registryV1Builder) extractMetadataDependencies() error {
	dependenciesData, err := fs.ReadFile(b.root, dependenciesFile)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	var dependencies struct {
		Dependencies []property.Property `json:"dependencies"`
	}
	if err := yaml.Unmarshal(dependenciesData, &dependencies, yaml.DisallowUnknownFields); err != nil {
		return fmt.Errorf("failed to extract dependencies from %q: %w", dependenciesFile, err)
	}
	b.dependencies = dependencies.Dependencies
	return nil
}

func (b *registryV1Builder) extractManifests() error {
	manifestsEntries, err := fs.ReadDir(b.root, manifestsDir)
	if err != nil {
		return err
	}
	var (
		validationErrors []error
		manifests        = make(map[string][]*unstructured.Unstructured)
	)
	for _, manifestsEntry := range manifestsEntries {
		path := filepath.Join(manifestsDir, manifestsEntry.Name())
		if err := func() error {
			if manifestsEntry.IsDir() {
				return fmt.Errorf("found manifests subdirectory %q; manifest subdirectories are forbidden", path)
			}

			manifestData, err := fs.ReadFile(b.root, path)
			if err != nil {
				return fmt.Errorf("failed to read manifest %q: %v", path, err)
			}

			res := resource.NewLocalBuilder().Flatten().Unstructured().Stream(bytes.NewReader(manifestData), path).Do()
			infos, err := res.Infos()
			if err != nil {
				return fmt.Errorf("failed to parse manifest %q: %v", err)
			}
			for _, info := range infos {
				obj := info.Object.(*unstructured.Unstructured)
				manifests[path] = append(manifests[path], obj)
				if obj.GroupVersionKind().Kind == "ClusterServiceVersion" {
					b.csv = &v1alpha1.ClusterServiceVersion{}
					if err := runtime.DefaultUnstructuredConverter.FromUnstructured(obj.Object, b.csv); err != nil {
						return err
					}
				}
			}
			return nil
		}(); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}
	if len(validationErrors) > 0 {
		return fmt.Errorf("failed to extract manifests from %q: %v", manifestsDir, err)
	}
	b.manifests = manifests
	return nil
}

func (b *registryV1Builder) validate() error {
	validations := []func() error{
		b.validateRelease,
		b.validateExtraLabels,
		b.validateExtraAnnotations,
		b.validateBundle,
	}
	var validationErrors []error
	for _, fn := range validations {
		if err := fn(); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}
	return errors.Join(validationErrors...)
}

const releasePattern = `^[A-Za-z0-9]+([.+-][A-Za-z0-9]+)*$`

var releaseRegex = regexp.MustCompile(releasePattern)

func (b *registryV1Builder) validateRelease() error {
	if b.release == "" {
		return errors.New("release must be specified")
	}
	if !releaseRegex.MatchString(b.release) {
		return fmt.Errorf("invalid release %q: does not match pattern %s", b.release, releasePattern)
	}
	return nil
}

func (b *registryV1Builder) validateExtraLabels() error {
	var validationErrors []error
	for k := range b.extraLabels {
		if len(k) == 0 {
			validationErrors = append(validationErrors, fmt.Errorf("invalid key %q: must not be empty", k))
		}
	}
	if len(validationErrors) > 0 {
		return fmt.Errorf("invalid extra labels: %v", errors.Join(validationErrors...))
	}
	return nil
}

func (b *registryV1Builder) validateExtraAnnotations() error {
	var validationErrors []error
	for k := range b.extraAnnotations {
		if len(k) == 0 {
			validationErrors = append(validationErrors, fmt.Errorf("invalid key %q: must not be empty", k))
		}
		if len(k) > 255 {
			validationErrors = append(validationErrors, fmt.Errorf("invalid key %q: must not exceed 255 characters", k))
		}
	}
	if len(validationErrors) > 0 {
		return fmt.Errorf("invalid extra annotations: %v", errors.Join(validationErrors...))
	}
	return nil
}

func (b *registryV1Builder) validateBundle() error {
	validations := []func() error{
		b.validateMetadata,
		b.validateManifests,
	}
	var validationErrors []error
	for _, fn := range validations {
		if err := fn(); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}
	return errors.Join(validationErrors...)
}

func (b *registryV1Builder) validateMetadata() error {
	validations := []func() error{
		b.validateMetadataAnnotations,
		b.validateMetadataProperties,
		b.validateMetadataDependencies,
	}
	var validationErrors []error
	for _, fn := range validations {
		if err := fn(); err != nil {
			validationErrors = append(validationErrors, err)
		}
	}
	return errors.Join(validationErrors...)
}

func (b *registryV1Builder) validateMetadataAnnotations() error {
	if err := func() error {
		if b.metadataAnnotations == nil {
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
			annotationManifests: requireEqual(manifestsDir + "/"),
			annotationMetadata:  requireEqual(metadataDir + "/"),
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
			value := b.metadataAnnotations[key]
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

func (b *registryV1Builder) validateMetadataProperties() error {
	// Once properties are parsed successfully, there are no further
	// aspects to validate.
	return nil
}

func (b *registryV1Builder) validateMetadataDependencies() error {
	if err := func() error {
		if b.dependencies == nil {
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
		for _, dependency := range b.dependencies {
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

func (b *registryV1Builder) validateManifests() error {
	var (
		validationErrors []error
		csvCount         int
	)
	for path, objects := range b.manifests {
		if len(objects) != 1 {
			validationErrors = append(validationErrors, fmt.Errorf("manifest %q contains %d objects, expected exactly 1", path, len(objects)))
		}
		for _, obj := range objects {
			if obj.GroupVersionKind().Kind == "ClusterServiceVersion" {
				csvCount++
			}
		}
	}
	if csvCount != 1 {
		validationErrors = append(validationErrors, fmt.Errorf("found %d ClusterServiceVersion objects, expected exactly 1", csvCount))
	}
	return errors.Join(validationErrors...)
}

const (
	mediaType = "registry+v1"

	manifestsDir = "manifests"
	metadataDir  = "metadata"

	annotationsFile  = metadataDir + "/annotations.yaml"
	propertiesFile   = metadataDir + "/properties.yaml"
	dependenciesFile = metadataDir + "/dependencies.yaml"

	annotationMediaType = "operators.operatorframework.io.bundle.mediatype.v1"
	annotationManifests = "operators.operatorframework.io.bundle.manifests.v1"
	annotationMetadata  = "operators.operatorframework.io.bundle.metadata.v1"
	annotationPackage   = "operators.operatorframework.io.bundle.package.v1"

	ociAnnotationName    = "kpm.operatorframework.io/name"
	ociAnnotationVersion = "kpm.operatorframework.io/version"
	ociAnnotationRelease = "kpm.operatorframework.io/release"
)

func loadRegistryV1Bytes(specData []byte, workingDir string) (builder.Builder, error) {
	var rv1Spec specsv1.RegistryV1
	if err := yaml.Unmarshal(specData, &rv1Spec, yaml.DisallowUnknownFields); err != nil {
		return nil, err
	}

	b := &registryV1Builder{
		release: rv1Spec.Release,
	}

	switch rv1Spec.Source.SourceType {
	case specsv1.RegistryV1SourceTypeBundleDirectory:
		b.root = os.DirFS(filepath.Join(workingDir, rv1Spec.Source.BundleDirectory.Path))
	default:
		return nil, fmt.Errorf("unknown source type: %q", rv1Spec.Source.SourceType)
	}
	return b, nil
}

func init() {
	if err := loader.DefaultRegistry.RegisterKind(specsv1.GroupVersion.WithKind(specsv1.KindRegistryV1), loadRegistryV1Bytes); err != nil {
		panic(err)
	}
}
