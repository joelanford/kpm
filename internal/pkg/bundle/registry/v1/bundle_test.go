package v1

import (
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/utils/ptr"
	"oras.land/oras-go/v2/content/memory"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
)

func Test_BundleFSLoader_Load(t *testing.T) {
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
metadata:
  name: example.v1.2.3
spec:
  version: "1.2.3"
  customresourcedefinitions:
    owned:
      - name: resources.group.example.com
        version: v1alpha1
        kind: Resource
`)},
				"manifests/crd.yaml": &fstest.MapFile{Data: []byte(`
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: resources.group.example.com
spec:
  names:
    kind: Resource
  versions:
    - name: v1alpha1
`)},
				"manifests/secret.yaml": &fstest.MapFile{Data: []byte(`
apiVersion: v1
kind: Secret
`)},
				"metadata/annotations.yaml": &fstest.MapFile{Data: []byte(`
annotations:
  operators.operatorframework.io.bundle.mediatype.v1: registry+v1
  operators.operatorframework.io.bundle.manifests.v1: manifests/
  operators.operatorframework.io.bundle.metadata.v1: metadata/
  operators.operatorframework.io.bundle.package.v1: example
`)},
				"metadata/properties.yaml":   &fstest.MapFile{Data: []byte(`properties: []`)},
				"metadata/dependencies.yaml": &fstest.MapFile{Data: []byte(`dependencies: []`)},
			},
			expected: &Bundle{
				manifests: &manifests{
					csv: newCSVFromData(t, "csv.yaml", []byte(`
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: example.v1.2.3
spec:
  version: "1.2.3"
  customresourcedefinitions:
    owned:
      - name: resources.group.example.com
        version: v1alpha1
        kind: Resource
`)),
					crds: []File[*apiextensionsv1.CustomResourceDefinition]{
						newCRDFromData(t, "crd.yaml", []byte(`
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
metadata:
  name: resources.group.example.com
spec:
  names:
    kind: Resource
  versions:
    - name: v1alpha1
`)),
					},
					others: []File[client.Object]{
						newObjectFromData[*corev1.Secret](t, "secret.yaml", []byte(`
apiVersion: v1
kind: Secret
`)),
					},
				},
				metadata: &metadata{
					annotationsFile: newFromData[Annotations](t, annotationsFileName, []byte(`
annotations:
  operators.operatorframework.io.bundle.mediatype.v1: registry+v1
  operators.operatorframework.io.bundle.manifests.v1: manifests/
  operators.operatorframework.io.bundle.metadata.v1: metadata/
  operators.operatorframework.io.bundle.package.v1: example
`)),
					propertiesFile:   ptr.To(newFromData[Properties](t, propertiesFileName, []byte(`properties: []`))),
					dependenciesFile: ptr.To(newFromData[Dependencies](t, dependenciesFileName, []byte(`dependencies: []`))),
				},
			},
			assertErr: require.NoError,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := NewBundleFSLoader(tt.fsys)
			actual, err := l.Load()
			tt.assertErr(t, err)
			require.Equal(t, tt.expected, actual)
		})
	}
}

func Test_Bundle_MarshalOCI(t *testing.T) {
	tests := []struct {
		name        string
		bundle      Bundle
		expected    ocispec.Descriptor
		assertErr   require.ErrorAssertionFunc
		shouldPanic bool
	}{
		{
			name: "succeeds",
			bundle: Bundle{
				manifests: &manifests{
					csv: newCSVFromData(t, "csv.yaml", []byte(`
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: example.v1.2.3
spec:
  version: "1.2.3"
`)),
					crds: []File[*apiextensionsv1.CustomResourceDefinition]{
						newCRDFromData(t, "crd.yaml", []byte(`
apiVersion: apiextensions.k8s.io/v1
kind: CustomResourceDefinition
`)),
					},
					others: []File[client.Object]{
						newObjectFromData[*corev1.Secret](t, "secret.yaml", []byte(`
apiVersion: v1
kind: Secret
`)),
					},
				},
				metadata: &metadata{
					annotationsFile: newFromData[Annotations](t, annotationsFileName, []byte(`
annotations:
  operators.operatorframework.io.bundle.mediatype.v1: registry+v1
  operators.operatorframework.io.bundle.manifests.v1: manifests/
  operators.operatorframework.io.bundle.metadata.v1: metadata/
  operators.operatorframework.io.bundle.package.v1: example
`)),
				},
			},
			expected: ocispec.Descriptor{
				MediaType: ocispec.MediaTypeImageManifest,
				// NOTE: DO NOT CHANGE THIS DIGEST if nothing in the bundle has changed.
				//   This ensures that bundles can always be rebuilt such that the digest
				//   never changes if input hasn't changed.
				//
				//   If you need to change something in the bundle, there should be a PR
				//   where the only changes are in the input/output of this test.
				Digest: digest.NewDigestFromEncoded(digest.SHA256, "40ba1bcad69d6e0d1e4be0c21d2ee68f3b90125c585b046ca8c172bb4c828203"),
				Size:   401,
			},
			assertErr: require.NoError,
		},
		{
			name:        "bundle zero-value",
			bundle:      Bundle{},
			shouldPanic: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target := memory.New()

			doIt := func() {
				actual, err := tt.bundle.MarshalOCI(t.Context(), target)
				tt.assertErr(t, err)
				require.Equal(t, tt.expected, actual)
			}

			if tt.shouldPanic {
				require.Panics(t, doIt)
			} else {
				require.NotPanics(t, doIt)
			}
		})
	}
}

func newCSVFromData(t *testing.T, filename string, data []byte) File[*v1alpha1.ClusterServiceVersion] {
	t.Helper()
	f, err := NewYAMLDataFile[*v1alpha1.ClusterServiceVersion](filename, data)
	require.NoError(t, err)
	return *f
}

func newCRDFromData(t *testing.T, filename string, data []byte) File[*apiextensionsv1.CustomResourceDefinition] {
	t.Helper()
	f, err := NewYAMLDataFile[*apiextensionsv1.CustomResourceDefinition](filename, data)
	require.NoError(t, err)
	return *f
}

func newObjectFromData[T client.Object](t *testing.T, filename string, data []byte) File[client.Object] {
	t.Helper()
	f, err := NewYAMLDataFile[T](filename, data)
	require.NoError(t, err)
	return NewPrecomputedFile[client.Object](filename, data, f.Value())
}

func newFromData[T any](t *testing.T, filename string, data []byte) File[T] {
	t.Helper()
	f, err := NewYAMLDataFile[T](filename, data)
	require.NoError(t, err)
	return *f
}
