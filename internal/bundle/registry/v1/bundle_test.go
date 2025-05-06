package v1

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
)

func TestLoadFS(t *testing.T) {
	type args struct {
		fsys fs.FS
	}
	tests := []struct {
		name      string
		fsys      fs.FS
		expected  *Bundle
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "succeeds",
			fsys: fstest.MapFS{
				"manifests/csv.yaml": &fstest.MapFile{Data: []byte(`
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
`)},
				"metadata/annotations.yaml": &fstest.MapFile{Data: []byte(`
annotations:
  operators.operatorframework.io.bundle.mediatype.v1: registry+v1
  operators.operatorframework.io.bundle.manifests.v1: manifests/
  operators.operatorframework.io.bundle.metadata.v1: metadata/
  operators.operatorframework.io.bundle.package.v1: example
`)},
			},
			expected: &Bundle{
				manifests: manifests{manifestFiles: []manifestFile{
					{
						filename: "csv.yaml",
						objects:  []*unstructured.Unstructured{makeUnstructured(v1alpha1.SchemeGroupVersion.WithKind("ClusterServiceVersion"))},
					},
				}},
				metadata: metadata{annotationsFile: Annotations{map[string]string{
					AnnotationMediaType: MediaType,
					AnnotationManifests: ManifestsDirectory,
					AnnotationMetadata:  MetadataDirectory,
					AnnotationPackage:   "example",
				}}},
				csv: v1alpha1.ClusterServiceVersion{
					TypeMeta: metav1.TypeMeta{APIVersion: v1alpha1.SchemeGroupVersion.String(), Kind: "ClusterServiceVersion"},
				},
			},
			assertErr: require.NoError,
		},
		{
			name:     "fails during load step",
			fsys:     fstest.MapFS{},
			expected: nil,
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "failed to load bundle:")
			},
		},
		{
			name: "fails during validate step",
			fsys: fstest.MapFS{
				"manifests/csv.yaml": &fstest.MapFile{Data: []byte(`
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
`)},
				"metadata/annotations.yaml": &fstest.MapFile{Data: []byte(`annotations: {}`)},
			},
			expected: nil,
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "failed to validate bundle:")
			},
		},
		{
			name: "fails during complete step",
			fsys: fstest.MapFS{
				"manifests/a.yaml": &fstest.MapFile{Data: []byte(`
apiVersion: v1
kind: ConfigMap
`)},
				"manifests/csv.yaml": &fstest.MapFile{Data: []byte(`
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
spec:
  version: "v1.2.3.4"
`)},
				"metadata/annotations.yaml": &fstest.MapFile{Data: []byte(`
annotations:
  operators.operatorframework.io.bundle.mediatype.v1: registry+v1
  operators.operatorframework.io.bundle.manifests.v1: manifests/
  operators.operatorframework.io.bundle.metadata.v1: metadata/
  operators.operatorframework.io.bundle.package.v1: example
`)},
			},
			expected: nil,
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "failed to complete bundle:")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			actual, err := LoadFS(tt.fsys)
			tt.assertErr(t, err)

			if actual != nil {
				actual.fsys = nil
				actual.manifests.fsys = nil
				actual.metadata.fsys = nil
			}
			require.Equal(t, tt.expected, actual)
		})
	}
}
