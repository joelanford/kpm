package v1

import (
	"encoding/json"
	"io/fs"
	"maps"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"
)

func Test_metadata_load(t *testing.T) {
	loadableAnnotations, _ := yaml.Marshal(map[string]any{"annotations": map[string]string{"foo": "bar"}})
	loadableProperties, _ := yaml.Marshal(map[string]any{"properties": []Property{{Type: "fizz", Value: json.RawMessage(`["buzz"]`)}}})
	loadableDependencies, _ := yaml.Marshal(map[string]any{"dependencies": []Property{{Type: "tic", Value: json.RawMessage(`{"tac":"toe"}`)}}})

	tests := []struct {
		name      string
		fsys      fs.FS
		expected  metadata
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "metadata loads minimum required files successfully",
			fsys: fstest.MapFS{
				AnnotationsFile: &fstest.MapFile{Data: loadableAnnotations},
			},
			expected: metadata{
				annotationsFile: Annotations{Annotations: map[string]string{"foo": "bar"}},
			},
			assertErr: require.NoError,
		},
		{
			name: "metadata loads all files successfully",
			fsys: fstest.MapFS{
				AnnotationsFile:  &fstest.MapFile{Data: loadableAnnotations},
				PropertiesFile:   &fstest.MapFile{Data: loadableProperties},
				DependenciesFile: &fstest.MapFile{Data: loadableDependencies},
			},
			expected: metadata{
				annotationsFile: Annotations{Annotations: map[string]string{"foo": "bar"}},
				propertiesFile: Properties{Properties: []Property{
					{Type: "fizz", Value: json.RawMessage(`["buzz"]`)},
				}},
				dependenciesFile: Dependencies{Dependencies: []Dependency{
					{Type: "tic", Value: json.RawMessage(`{"tac":"toe"}`)},
				}},
			},
			assertErr: require.NoError,
		},
		{
			name: "metadata load fails due to missing annotations",
			fsys: fstest.MapFS{},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "open annotations.yaml: file does not exist")
			},
		},
		{
			name: "metadata load fails due to malformed annotations",
			fsys: fstest.MapFS{
				AnnotationsFile: &fstest.MapFile{Data: []byte(`{"foo":"bar"}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `failed to load annotations from "annotations.yaml"`)
				require.ErrorContains(t, err, `unknown field "foo"`)
			},
		},
		{
			name: "metadata load fails due to malformed properties",
			fsys: fstest.MapFS{
				AnnotationsFile: &fstest.MapFile{Data: loadableAnnotations},
				PropertiesFile:  &fstest.MapFile{Data: []byte(`{"foo":"bar"}`)},
			},
			expected: metadata{
				annotationsFile: Annotations{Annotations: map[string]string{"foo": "bar"}},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `failed to load properties from "properties.yaml"`)
				require.ErrorContains(t, err, `unknown field "foo"`)
			},
		},
		{
			name: "metadata load fails due to malformed dependencies",
			fsys: fstest.MapFS{
				AnnotationsFile:  &fstest.MapFile{Data: loadableAnnotations},
				DependenciesFile: &fstest.MapFile{Data: []byte(`{"foo":"bar"}`)},
			},
			expected: metadata{
				annotationsFile: Annotations{Annotations: map[string]string{"foo": "bar"}},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `failed to load dependencies from "dependencies.yaml"`)
				require.ErrorContains(t, err, `unknown field "foo"`)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &metadata{fsys: tt.fsys}
			err := m.load()
			tt.assertErr(t, err)

			require.Equal(t, tt.expected.annotationsFile, m.annotationsFile)
			require.Equal(t, tt.expected.propertiesFile, m.propertiesFile)
			require.Equal(t, tt.expected.dependenciesFile, m.dependenciesFile)
		})
	}
}

func Test_metadata_validateAnnotations(t *testing.T) {
	validAnnotations := map[string]string{
		AnnotationMediaType: MediaType,
		AnnotationManifests: ManifestsDirectory,
		AnnotationMetadata:  MetadataDirectory,
		AnnotationPackage:   "example",
	}

	tests := []struct {
		name            string
		annotationsFile Annotations
		assertErr       require.ErrorAssertionFunc
	}{
		{
			name: "valid annotations",
			annotationsFile: Annotations{
				Annotations: validAnnotations,
			},
			assertErr: require.NoError,
		},
		{
			name:            "zero annotations is invalid",
			annotationsFile: Annotations{},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "no annotations found")
			},
		},
		{
			name: "missing media type",
			annotationsFile: Annotations{
				Annotations: mapWithoutKey(validAnnotations, AnnotationMediaType),
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `required key "operators.operatorframework.io.bundle.mediatype.v1" not found`)
			},
		},
		{
			name: "missing manifests directory",
			annotationsFile: Annotations{
				Annotations: mapWithoutKey(validAnnotations, AnnotationManifests),
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `required key "operators.operatorframework.io.bundle.manifests.v1" not found`)
			},
		},
		{
			name: "missing metadata directory",
			annotationsFile: Annotations{
				Annotations: mapWithoutKey(validAnnotations, AnnotationMetadata),
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `required key "operators.operatorframework.io.bundle.metadata.v1" not found`)
			},
		},
		{
			name: "missing package",
			annotationsFile: Annotations{
				Annotations: mapWithoutKey(validAnnotations, AnnotationPackage),
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `required key "operators.operatorframework.io.bundle.package.v1" not found`)
			},
		},
		{
			name: "invalid media type",
			annotationsFile: Annotations{
				Annotations: mapWithKeyValue(validAnnotations, AnnotationMediaType, "invalid"),
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `invalid value for annotation key "operators.operatorframework.io.bundle.mediatype.v1": requires value "registry+v1"`)
			},
		},
		{
			name: "invalid manifests directory",
			annotationsFile: Annotations{
				Annotations: mapWithKeyValue(validAnnotations, AnnotationManifests, "invalid"),
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `invalid value for annotation key "operators.operatorframework.io.bundle.manifests.v1": requires value "manifests/"`)
			},
		},
		{
			name: "invalid metadata directory",
			annotationsFile: Annotations{
				Annotations: mapWithKeyValue(validAnnotations, AnnotationMetadata, "invalid"),
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `invalid value for annotation key "operators.operatorframework.io.bundle.metadata.v1": requires value "metadata/"`)
			},
		},
		{
			name: "invalid package",
			annotationsFile: Annotations{
				Annotations: mapWithKeyValue(validAnnotations, AnnotationPackage, "$package"),
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `invalid value for annotation key "operators.operatorframework.io.bundle.package.v1"`)
				require.ErrorContains(t, err, "RFC 1123 subdomain")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &metadata{
				annotationsFile: tt.annotationsFile,
			}
			err := m.validateAnnotations()
			tt.assertErr(t, err)
		})
	}
}

func Test_metadata_validateProperties(t *testing.T) {
	tests := []struct {
		name           string
		propertiesFile Properties
		assertErr      require.ErrorAssertionFunc
	}{
		{
			name:           "empty properties is valid",
			propertiesFile: Properties{},
			assertErr:      require.NoError,
		},
		{
			name: "arbitrary properties are valid",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: "a", Value: []byte(`null`)},
					{Type: "b", Value: []byte(`0`)},
					{Type: "c", Value: []byte(`1.1`)},
					{Type: "d", Value: []byte(`"hello world"`)},
					{Type: "e", Value: []byte(`[]`)},
					{Type: "f", Value: []byte(`{}`)},
				},
			},
			assertErr: require.NoError,
		},
		{
			name: "duplicate properties are valid",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: "a", Value: []byte(`[1]`)},
					{Type: "a", Value: []byte(`[2]`)},
				},
			},
			assertErr: require.NoError,
		},
		{
			name: "using property variants of dependencies is valid",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: TypePropertyPackageRequired, Value: []byte(`{"packageName":"foo","versionRange":"<=1.2.3"}`)},
					{Type: TypePropertyGVKRequired, Value: []byte(`{"group":"example.com","version":"v1","kind":"Foo"}`)},
				},
			},
			assertErr: require.NoError,
		},
		{
			name: "use of reserved olm.package is invalid",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: TypePropertyPackage, Value: []byte(`{"packageName":"foo","version":"1.2.3"}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "found reserved properties")
			},
		},
		{
			name: "use of reserved olm.gvk is invalid",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: TypePropertyGVK, Value: []byte(`{"group":"example.com","version":"v1","kind":"Foo"}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "found reserved properties")
			},
		},
		{
			name: "olm.package.required must have package name",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: TypePropertyPackageRequired, Value: []byte(`{"packageName":"","versionRange":"<=1.2.3"}`)},
					{Type: TypePropertyPackageRequired, Value: []byte(`{"versionRange":"<=1.2.3"}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `failed to validate value "{\"packageName\":\"\",\"versionRange\":\"<=1.2.3\"}": packageName is required`)
				require.ErrorContains(t, err, `failed to validate value "{\"versionRange\":\"<=1.2.3\"}": packageName is required`)
			},
		},
		{
			name: "olm.package.required package name must be a DNS 1123 subdomain",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: TypePropertyPackageRequired, Value: []byte(`{"packageName":"$foo","versionRange":"<=1.2.3"}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `packageName "$foo" is invalid: a lowercase RFC 1123 subdomain must consist of`)
			},
		},
		{
			name: "olm.package.required must have version range",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: TypePropertyPackageRequired, Value: []byte(`{"packageName":"foo","versionRange":""}`)},
					{Type: TypePropertyPackageRequired, Value: []byte(`{"packageName":"foo"}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `failed to validate value "{\"packageName\":\"foo\",\"versionRange\":\"\"}": versionRange is required`)
				require.ErrorContains(t, err, `failed to validate value "{\"packageName\":\"foo\"}": versionRange is required`)
			},
		},
		{
			name: "olm.package.required version range must be valid",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: TypePropertyPackageRequired, Value: []byte(`{"packageName":"foo","versionRange":"foobar"}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `versionRange "foobar" is invalid`)
			},
		},
		{
			name: "olm.gvk.required must have group, version, and kind",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: TypePropertyGVKRequired, Value: []byte(`{}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `group is required`)
				require.ErrorContains(t, err, `version is required`)
				require.ErrorContains(t, err, `kind is required`)
			},
		},
		{
			name: "olm.gvk.required must have valid group, version, and kind",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: TypePropertyGVKRequired, Value: []byte(`{"group":"$foo","version":"bar", "kind":"baz"}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `group "$foo" is invalid: a lowercase RFC 1123 subdomain must consist`)
				require.ErrorContains(t, err, `version "bar" is invalid: must match pattern`)
				require.ErrorContains(t, err, `kind "baz" is invalid: must match pattern`)
			},
		},
		{
			name: "property types must be set",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: "", Value: []byte(`null`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `property at index 0 with type "" is invalid: type is required`)
			},
		},
		{
			name: "empty property values are invalid",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: "a", Value: []byte(``)},
					{Type: "b", Value: nil},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `property at index 0 with type "a" is invalid: value is required`)
				require.ErrorContains(t, err, `property at index 1 with type "b" is invalid: value is required`)
			},
		},
		{
			name: "non-JSON property values are invalid",
			propertiesFile: Properties{
				Properties: []Property{
					{Type: "a", Value: []byte(`}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `property at index 0 with type "a" is invalid: failed to unmarshal value`)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &metadata{
				propertiesFile: tt.propertiesFile,
			}
			err := m.validateProperties()
			tt.assertErr(t, err)
		})
	}
}

func Test_metadata_validateDependencies(t *testing.T) {
	tests := []struct {
		name             string
		dependenciesFile Dependencies
		assertErr        require.ErrorAssertionFunc
	}{
		{
			name:             "empty dependencies is valid",
			dependenciesFile: Dependencies{},
			assertErr:        require.NoError,
		},
		{
			name: "known dependencies are valid",
			dependenciesFile: Dependencies{
				Dependencies: []Dependency{
					{Type: TypeDependencyPackage, Value: []byte(`{"packageName":"foo","version":"1.2.3"}`)},
					{Type: TypeDependencyGVK, Value: []byte(`{"group":"example.com","version":"v1","kind":"Foo"}`)},
				},
			},
			assertErr: require.NoError,
		},
		{
			name: "duplicate dependencies of the same type are valid",
			dependenciesFile: Dependencies{
				Dependencies: []Dependency{
					{Type: TypeDependencyPackage, Value: []byte(`{"packageName":"foo","version":"1.2.3"}`)},
					{Type: TypeDependencyPackage, Value: []byte(`{"packageName":"bar","version":"1.2.3"}`)},
					{Type: TypeDependencyGVK, Value: []byte(`{"group":"example.com","version":"v1","kind":"Foo"}`)},
					{Type: TypeDependencyGVK, Value: []byte(`{"group":"example.com","version":"v1","kind":"Bar"}`)},
				},
			},
			assertErr: require.NoError,
		},
		{
			name: "dependency types must be set",
			dependenciesFile: Dependencies{
				Dependencies: []Dependency{
					{Type: "", Value: []byte(`null`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `type is required`)
			},
		},
		{
			name: "unknown dependency types are invalid",
			dependenciesFile: Dependencies{
				Dependencies: []Dependency{
					{Type: "a", Value: []byte(`null`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `dependency at index 0 with type "a" is invalid: unknown type`)
			},
		},
		{
			name: "empty dependency values are invalid",
			dependenciesFile: Dependencies{
				Dependencies: []Dependency{
					{Type: TypeDependencyPackage, Value: []byte(``)},
					{Type: TypeDependencyPackage, Value: nil},
					{Type: TypeDependencyGVK, Value: []byte(``)},
					{Type: TypeDependencyGVK, Value: nil},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `dependency at index 0 with type "olm.package" is invalid: value is required`)
				require.ErrorContains(t, err, `dependency at index 1 with type "olm.package" is invalid: value is required`)
				require.ErrorContains(t, err, `dependency at index 2 with type "olm.gvk" is invalid: value is required`)
				require.ErrorContains(t, err, `dependency at index 3 with type "olm.gvk" is invalid: value is required`)
			},
		},
		{
			name: "non-JSON dependency values are invalid",
			dependenciesFile: Dependencies{
				Dependencies: []Dependency{
					{Type: TypeDependencyPackage, Value: []byte(`}`)},
					{Type: TypeDependencyGVK, Value: []byte(`}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `dependency at index 0 with type "olm.package" is invalid: failed to unmarshal value`)
				require.ErrorContains(t, err, `dependency at index 1 with type "olm.gvk" is invalid: failed to unmarshal value`)
			},
		},
		{
			name: "olm.package must have package name",
			dependenciesFile: Dependencies{
				Dependencies: []Dependency{
					{Type: TypeDependencyPackage, Value: []byte(`{"packageName":"","version":"<=1.2.3"}`)},
					{Type: TypeDependencyPackage, Value: []byte(`{"version":"<=1.2.3"}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `failed to validate value "{\"packageName\":\"\",\"version\":\"<=1.2.3\"}": packageName is required`)
				require.ErrorContains(t, err, `failed to validate value "{\"version\":\"<=1.2.3\"}": packageName is required`)
			},
		},
		{
			name: "olm.package package name must be a DNS 1123 subdomain",
			dependenciesFile: Dependencies{
				Dependencies: []Dependency{
					{Type: TypeDependencyPackage, Value: []byte(`{"packageName":"$foo","version":"<=1.2.3"}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `packageName "$foo" is invalid: a lowercase RFC 1123 subdomain must consist of`)
			},
		},
		{
			name: "olm.package must have version range",
			dependenciesFile: Dependencies{
				Dependencies: []Dependency{
					{Type: TypeDependencyPackage, Value: []byte(`{"packageName":"foo","version":""}`)},
					{Type: TypeDependencyPackage, Value: []byte(`{"packageName":"foo"}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `failed to validate value "{\"packageName\":\"foo\",\"version\":\"\"}": version is required`)
				require.ErrorContains(t, err, `failed to validate value "{\"packageName\":\"foo\"}": version is required`)
			},
		},
		{
			name: "olm.package version range must be valid",
			dependenciesFile: Dependencies{
				Dependencies: []Dependency{
					{Type: TypeDependencyPackage, Value: []byte(`{"packageName":"foo","version":"foobar"}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `version "foobar" is invalid`)
			},
		},
		{
			name: "olm.gvk must have group, version, and kind",
			dependenciesFile: Dependencies{
				Dependencies: []Dependency{
					{Type: TypeDependencyGVK, Value: []byte(`{}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `group is required`)
				require.ErrorContains(t, err, `version is required`)
				require.ErrorContains(t, err, `kind is required`)
			},
		},
		{
			name: "olm.gvk must have valid group, version, and kind",
			dependenciesFile: Dependencies{
				Dependencies: []Dependency{
					{Type: TypeDependencyGVK, Value: []byte(`{"group":"$foo","version":"bar", "kind":"baz"}`)},
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `group "$foo" is invalid: a lowercase RFC 1123 subdomain must consist`)
				require.ErrorContains(t, err, `version "bar" is invalid: must match pattern`)
				require.ErrorContains(t, err, `kind "baz" is invalid: must match pattern`)
			},
		},
		// TODO: Add a bunch of tests for invalid input.
		//   1. Make sure that validator type assertion is working as expected
		//   2. Consider adding more validators (beyond "does it parse" for dependencies)
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &metadata{
				dependenciesFile: tt.dependenciesFile,
			}
			err := m.validateDependencies()
			tt.assertErr(t, err)
		})
	}
}

func Test_metadata_validate(t *testing.T) {
	tests := []struct {
		name      string
		metadata  metadata
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "passes all validations",
			metadata: metadata{
				annotationsFile: Annotations{Annotations: map[string]string{
					AnnotationMediaType: MediaType,
					AnnotationManifests: ManifestsDirectory,
					AnnotationMetadata:  MetadataDirectory,
					AnnotationPackage:   "example",
				}},
			},
			assertErr: require.NoError,
		},
		{
			name: "validate collects suberrors",
			metadata: metadata{
				annotationsFile:  Annotations{Annotations: map[string]string{"foo": "bar"}},
				propertiesFile:   Properties{Properties: []Property{{Type: "a"}}},
				dependenciesFile: Dependencies{Dependencies: []Dependency{{Type: TypeDependencyPackage}}},
			}, assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "invalid annotations")
				require.ErrorContains(t, err, "invalid properties")
				require.ErrorContains(t, err, "invalid dependencies")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.metadata.validate()
			tt.assertErr(t, err)
		})
	}
}

func mapWithoutKey[K comparable, V any](m map[K]V, key K) map[K]V {
	out := maps.Clone(m)
	delete(out, key)
	return out
}

func mapWithKeyValue[K comparable, V any](m map[K]V, k K, v V) map[K]V {
	out := maps.Clone(m)
	if out == nil {
		out = map[K]V{}
	}
	out[k] = v
	return out
}
