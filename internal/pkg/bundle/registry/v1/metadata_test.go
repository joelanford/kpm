package v1

import (
	"io/fs"
	"maps"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	"k8s.io/utils/ptr"
)

func Test_metadataFSLoader_loadMetadata(t *testing.T) {
	tests := []struct {
		name      string
		fsys      fs.FS
		expected  *metadata
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "loads minimum metadata successfully",
			fsys: fstest.MapFS{
				annotationsFileName: &fstest.MapFile{Data: []byte(`annotations: {}`)},
			},
			expected: &metadata{
				annotationsFile: NewPrecomputedFile[Annotations](annotationsFileName, []byte(`annotations: {}`), Annotations{Annotations: map[string]string{}}),
			},
			assertErr: require.NoError,
		},
		{
			name: "loads all metadata successfully",
			fsys: fstest.MapFS{
				annotationsFileName:  &fstest.MapFile{Data: []byte(`annotations: {"foo": "bar"}`)},
				propertiesFileName:   &fstest.MapFile{Data: []byte(`properties: [{"type":"a", "value":[]}]`)},
				dependenciesFileName: &fstest.MapFile{Data: []byte(`dependencies: [{"type":"b", "value":{}}]`)},
			},
			expected: &metadata{
				annotationsFile: NewPrecomputedFile[Annotations](annotationsFileName, []byte(`annotations: {"foo": "bar"}`),
					Annotations{Annotations: map[string]string{"foo": "bar"}}),
				propertiesFile: ptr.To(NewPrecomputedFile[Properties](propertiesFileName, []byte(`properties: [{"type":"a", "value":[]}]`),
					Properties{Properties: []Property{{Type: "a", Value: []byte(`[]`)}}})),
				dependenciesFile: ptr.To(NewPrecomputedFile[Dependencies](dependenciesFileName, []byte(`dependencies: [{"type":"b", "value":{}}]`),
					Dependencies{Dependencies: []Dependency{{Type: "b", Value: []byte(`{}`)}}})),
			},
			assertErr: require.NoError,
		},
		{
			name: "fails due to invalid yaml",
			fsys: fstest.MapFS{
				"annotations.yaml":  &fstest.MapFile{Data: []byte(`}`)},
				"properties.yaml":   &fstest.MapFile{Data: []byte(`}`)},
				"dependencies.yaml": &fstest.MapFile{Data: []byte(`}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "error parsing annotations.yaml")
				require.ErrorContains(t, err, "error parsing properties.yaml")
				require.ErrorContains(t, err, "error parsing dependencies.yaml")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := metadataFSLoader{fsys: tt.fsys}
			metadata, err := m.loadMetadata()
			tt.assertErr(t, err)
			require.Equal(t, tt.expected, metadata)
		})
	}
}

func Test_Metadata_Validate(t *testing.T) {
	tests := []struct {
		name      string
		metadata  metadata
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "passes all validations",
			metadata: metadata{
				annotationsFile: newAnnotationsFile(map[string]string{
					annotationMediaType: mediaType,
					annotationManifests: manifestsDirectory,
					annotationMetadata:  metadataDirectory,
					annotationPackage:   "example",
				}),
			},
			assertErr: require.NoError,
		},
		{
			name: "validate collects suberrors",
			metadata: metadata{
				annotationsFile:  newAnnotationsFile(map[string]string{"foo": "bar"}),
				propertiesFile:   newPropertiesFile([]Property{{Type: "a"}}),
				dependenciesFile: newDependenciesFile([]Dependency{{Type: typeDependencyPackage}}),
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

func Test_Metadata_validateAnnotations(t *testing.T) {
	validAnnotations := map[string]string{
		annotationMediaType: mediaType,
		annotationManifests: manifestsDirectory,
		annotationMetadata:  metadataDirectory,
		annotationPackage:   "example",
	}

	tests := []struct {
		name        string
		annotations map[string]string
		assertErr   require.ErrorAssertionFunc
	}{
		{
			name:        "valid annotations",
			annotations: validAnnotations,
			assertErr:   require.NoError,
		},
		{
			name:        "zero annotations is invalid",
			annotations: map[string]string{},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "no annotations found")
			},
		},
		{
			name:        "missing media type",
			annotations: mapWithoutKey(validAnnotations, annotationMediaType),
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `required key "operators.operatorframework.io.bundle.mediatype.v1" not found`)
			},
		},
		{
			name:        "missing manifests directory",
			annotations: mapWithoutKey(validAnnotations, annotationManifests),
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `required key "operators.operatorframework.io.bundle.manifests.v1" not found`)
			},
		},
		{
			name:        "missing metadata directory",
			annotations: mapWithoutKey(validAnnotations, annotationMetadata),
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `required key "operators.operatorframework.io.bundle.metadata.v1" not found`)
			},
		},
		{
			name:        "missing package",
			annotations: mapWithoutKey(validAnnotations, annotationPackage),
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `required key "operators.operatorframework.io.bundle.package.v1" not found`)
			},
		},
		{
			name:        "invalid media type",
			annotations: mapWithKeyValue(validAnnotations, annotationMediaType, "invalid"),
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `invalid value for annotation key "operators.operatorframework.io.bundle.mediatype.v1": requires value "registry+v1"`)
			},
		},
		{
			name:        "invalid manifests directory",
			annotations: mapWithKeyValue(validAnnotations, annotationManifests, "invalid"),
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `invalid value for annotation key "operators.operatorframework.io.bundle.manifests.v1": requires value "manifests/"`)
			},
		},
		{
			name:        "invalid metadata directory",
			annotations: mapWithKeyValue(validAnnotations, annotationMetadata, "invalid"),
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `invalid value for annotation key "operators.operatorframework.io.bundle.metadata.v1": requires value "metadata/"`)
			},
		},
		{
			name:        "invalid package",
			annotations: mapWithKeyValue(validAnnotations, annotationPackage, "$package"),
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `invalid value for annotation key "operators.operatorframework.io.bundle.package.v1"`)
				require.ErrorContains(t, err, "RFC 1123 subdomain")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &metadata{
				annotationsFile: newAnnotationsFile(tt.annotations),
			}
			err := m.validateAnnotations()
			tt.assertErr(t, err)
		})
	}
}

func Test_Metadata_validateProperties(t *testing.T) {
	tests := []struct {
		name       string
		properties []Property
		assertErr  require.ErrorAssertionFunc
	}{
		{
			name:       "empty properties is valid",
			properties: []Property{},
			assertErr:  require.NoError,
		},
		{
			name: "arbitrary properties are valid",
			properties: []Property{
				{Type: "a", Value: []byte(`null`)},
				{Type: "b", Value: []byte(`0`)},
				{Type: "c", Value: []byte(`1.1`)},
				{Type: "d", Value: []byte(`"hello world"`)},
				{Type: "e", Value: []byte(`[]`)},
				{Type: "f", Value: []byte(`{}`)},
			},
			assertErr: require.NoError,
		},
		{
			name: "duplicate properties are valid",
			properties: []Property{
				{Type: "a", Value: []byte(`[1]`)},
				{Type: "a", Value: []byte(`[2]`)},
			},
			assertErr: require.NoError,
		},
		{
			name: "using property variants of dependencies is valid",
			properties: []Property{
				{Type: typePropertyPackageRequired, Value: []byte(`{"packageName":"foo","versionRange":"<=1.2.3"}`)},
				{Type: typePropertyGVKRequired, Value: []byte(`{"group":"example.com","version":"v1","kind":"Foo"}`)},
			},
			assertErr: require.NoError,
		},
		{
			name: "use of reserved olm.package is invalid",
			properties: []Property{
				{Type: typePropertyPackage, Value: []byte(`{"packageName":"foo","version":"1.2.3"}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "found reserved properties")
			},
		},
		{
			name: "use of reserved olm.gvk is invalid",
			properties: []Property{
				{Type: typePropertyGVK, Value: []byte(`{"group":"example.com","version":"v1","kind":"Foo"}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "found reserved properties")
			},
		},
		{
			name: "olm.package.required must have package name",
			properties: []Property{
				{Type: typePropertyPackageRequired, Value: []byte(`{"packageName":"","versionRange":"<=1.2.3"}`)},
				{Type: typePropertyPackageRequired, Value: []byte(`{"versionRange":"<=1.2.3"}`)},
			},

			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `failed to validate value "{\"packageName\":\"\",\"versionRange\":\"<=1.2.3\"}": packageName is required`)
				require.ErrorContains(t, err, `failed to validate value "{\"versionRange\":\"<=1.2.3\"}": packageName is required`)
			},
		},
		{
			name: "olm.package.required package name must be a DNS 1123 subdomain",
			properties: []Property{
				{Type: typePropertyPackageRequired, Value: []byte(`{"packageName":"$foo","versionRange":"<=1.2.3"}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `packageName "$foo" is invalid: a lowercase RFC 1123 subdomain must consist of`)
			},
		},
		{
			name: "olm.package.required must have version range",
			properties: []Property{
				{Type: typePropertyPackageRequired, Value: []byte(`{"packageName":"foo","versionRange":""}`)},
				{Type: typePropertyPackageRequired, Value: []byte(`{"packageName":"foo"}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `failed to validate value "{\"packageName\":\"foo\",\"versionRange\":\"\"}": versionRange is required`)
				require.ErrorContains(t, err, `failed to validate value "{\"packageName\":\"foo\"}": versionRange is required`)
			},
		},
		{
			name: "olm.package.required version range must be valid",
			properties: []Property{
				{Type: typePropertyPackageRequired, Value: []byte(`{"packageName":"foo","versionRange":"foobar"}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `versionRange "foobar" is invalid`)
			},
		},
		{
			name: "olm.gvk.required must have group, version, and kind",
			properties: []Property{
				{Type: typePropertyGVKRequired, Value: []byte(`{}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `group is required`)
				require.ErrorContains(t, err, `version is required`)
				require.ErrorContains(t, err, `kind is required`)
			},
		},
		{
			name: "olm.gvk.required must have valid group, version, and kind",
			properties: []Property{
				{Type: typePropertyGVKRequired, Value: []byte(`{"group":"$foo","version":"bar", "kind":"baz"}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `group "$foo" is invalid: a lowercase RFC 1123 subdomain must consist`)
				require.ErrorContains(t, err, `version "bar" is invalid: must match pattern`)
				require.ErrorContains(t, err, `kind "baz" is invalid: must match pattern`)
			},
		},
		{
			name: "property types must be set",
			properties: []Property{
				{Type: "", Value: []byte(`null`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `property at index 0 with type "" is invalid: type is required`)
			},
		},
		{
			name: "empty property values are invalid",
			properties: []Property{
				{Type: "a", Value: []byte(``)},
				{Type: "b", Value: nil},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `property at index 0 with type "a" is invalid: value is required`)
				require.ErrorContains(t, err, `property at index 1 with type "b" is invalid: value is required`)
			},
		},
		{
			name: "non-JSON property values are invalid",
			properties: []Property{
				{Type: "a", Value: []byte(`}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `property at index 0 with type "a" is invalid: failed to unmarshal value`)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &metadata{
				propertiesFile: newPropertiesFile(tt.properties),
			}
			err := m.validateProperties()
			tt.assertErr(t, err)
		})
	}
}

func Test_Metadata_validateDependencies(t *testing.T) {
	tests := []struct {
		name         string
		dependencies []Dependency
		assertErr    require.ErrorAssertionFunc
	}{
		{
			name:         "empty dependencies is valid",
			dependencies: []Dependency{},
			assertErr:    require.NoError,
		},
		{
			name: "known dependencies are valid",
			dependencies: []Dependency{
				{Type: typeDependencyPackage, Value: []byte(`{"packageName":"foo","version":"1.2.3"}`)},
				{Type: typeDependencyGVK, Value: []byte(`{"group":"example.com","version":"v1","kind":"Foo"}`)},
			},
			assertErr: require.NoError,
		},
		{
			name: "duplicate dependencies of the same type are valid",
			dependencies: []Dependency{
				{Type: typeDependencyPackage, Value: []byte(`{"packageName":"foo","version":"1.2.3"}`)},
				{Type: typeDependencyPackage, Value: []byte(`{"packageName":"bar","version":"1.2.3"}`)},
				{Type: typeDependencyGVK, Value: []byte(`{"group":"example.com","version":"v1","kind":"Foo"}`)},
				{Type: typeDependencyGVK, Value: []byte(`{"group":"example.com","version":"v1","kind":"Bar"}`)},
			},
			assertErr: require.NoError,
		},
		{
			name: "dependency types must be set",
			dependencies: []Dependency{
				{Type: "", Value: []byte(`null`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `type is required`)
			},
		},
		{
			name: "unknown dependency types are invalid",
			dependencies: []Dependency{
				{Type: "a", Value: []byte(`null`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `dependency at index 0 with type "a" is invalid: unknown type`)
			},
		},
		{
			name: "empty dependency values are invalid",
			dependencies: []Dependency{
				{Type: typeDependencyPackage, Value: []byte(``)},
				{Type: typeDependencyPackage, Value: nil},
				{Type: typeDependencyGVK, Value: []byte(``)},
				{Type: typeDependencyGVK, Value: nil},
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
			dependencies: []Dependency{
				{Type: typeDependencyPackage, Value: []byte(`}`)},
				{Type: typeDependencyGVK, Value: []byte(`}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `dependency at index 0 with type "olm.package" is invalid: failed to unmarshal value`)
				require.ErrorContains(t, err, `dependency at index 1 with type "olm.gvk" is invalid: failed to unmarshal value`)
			},
		},
		{
			name: "olm.package must have package name",
			dependencies: []Dependency{
				{Type: typeDependencyPackage, Value: []byte(`{"packageName":"","version":"<=1.2.3"}`)},
				{Type: typeDependencyPackage, Value: []byte(`{"version":"<=1.2.3"}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `failed to validate value "{\"packageName\":\"\",\"version\":\"<=1.2.3\"}": packageName is required`)
				require.ErrorContains(t, err, `failed to validate value "{\"version\":\"<=1.2.3\"}": packageName is required`)
			},
		},
		{
			name: "olm.package package name must be a DNS 1123 subdomain",
			dependencies: []Dependency{
				{Type: typeDependencyPackage, Value: []byte(`{"packageName":"$foo","version":"<=1.2.3"}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `packageName "$foo" is invalid: a lowercase RFC 1123 subdomain must consist of`)
			},
		},
		{
			name: "olm.package must have version range",
			dependencies: []Dependency{
				{Type: typeDependencyPackage, Value: []byte(`{"packageName":"foo","version":""}`)},
				{Type: typeDependencyPackage, Value: []byte(`{"packageName":"foo"}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `failed to validate value "{\"packageName\":\"foo\",\"version\":\"\"}": version is required`)
				require.ErrorContains(t, err, `failed to validate value "{\"packageName\":\"foo\"}": version is required`)
			},
		},
		{
			name: "olm.package version range must be valid",
			dependencies: []Dependency{
				{Type: typeDependencyPackage, Value: []byte(`{"packageName":"foo","version":"foobar"}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `version "foobar" is invalid`)
			},
		},
		{
			name: "olm.gvk must have group, version, and kind",
			dependencies: []Dependency{
				{Type: typeDependencyGVK, Value: []byte(`{}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `group is required`)
				require.ErrorContains(t, err, `version is required`)
				require.ErrorContains(t, err, `kind is required`)
			},
		},
		{
			name: "olm.gvk must have valid group, version, and kind",
			dependencies: []Dependency{
				{Type: typeDependencyGVK, Value: []byte(`{"group":"$foo","version":"bar", "kind":"baz"}`)},
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
				dependenciesFile: newDependenciesFile(tt.dependencies),
			}
			err := m.validateDependencies()
			tt.assertErr(t, err)
		})
	}
}

func newAnnotationsFile(annotations map[string]string) AnnotationsFile {
	return NewPrecomputedFile[Annotations](annotationsFileName, nil, Annotations{Annotations: annotations})
}

func newPropertiesFile(properties []Property) *PropertiesFile {
	f := NewPrecomputedFile[Properties](propertiesFileName, nil, Properties{Properties: properties})
	return &f
}

func newDependenciesFile(dependencies []Dependency) *DependenciesFile {
	f := NewPrecomputedFile[Dependencies](dependenciesFileName, nil, Dependencies{Dependencies: dependencies})
	return &f
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
