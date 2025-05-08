package v1

import (
	"fmt"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"
)

func Test_manifests_load(t *testing.T) {
	tests := []struct {
		name      string
		fsys      fs.FS
		expected  []manifestFile
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "loads successfully",
			fsys: fstest.MapFS{
				"csv.yaml": &fstest.MapFile{
					Data: []byte(`
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
`),
				},
			},
			expected: []manifestFile{
				{filename: "csv.yaml", objects: []client.Object{&v1alpha1.ClusterServiceVersion{TypeMeta: metav1.TypeMeta{Kind: "ClusterServiceVersion", APIVersion: "operators.coreos.com/v1alpha1"}}}},
			},
			assertErr: require.NoError,
		},
		{
			name: "fails due to invalid yaml",
			fsys: fstest.MapFS{
				"invalid.yaml": &fstest.MapFile{
					Data: []byte(`}`),
				},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "error parsing invalid.yaml")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manifests{fsys: tt.fsys}
			err := m.load()
			tt.assertErr(t, err)
			require.Equal(t, tt.expected, m.manifestFiles)
		})
	}
}

func Test_manifests_validateNoSubDirectories(t *testing.T) {
	tests := []struct {
		name          string
		manifestFiles []manifestFile
		assertErr     require.ErrorAssertionFunc
	}{
		{
			name: "no sub directories, no error",
			manifestFiles: []manifestFile{
				{filename: "manifest1.yaml"},
				{filename: "manifest2.yaml"},
			},
			assertErr: require.NoError,
		},
		{
			name: "sub directory presence causes error",
			manifestFiles: []manifestFile{
				{filename: "manifest1.yaml"},
				{filename: "manifest2.yaml"},
				{filename: "subdir1/manifest10.yaml"},
				{filename: "subdir1/manifest11.yaml"},
				{filename: "subdir2/manifest20.yaml"},
				{filename: "subdir2/manifest21.yaml"},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "subdirectories not allowed")
				require.ErrorContains(t, err, "subdir1")
				require.ErrorContains(t, err, "subdir2")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manifests{
				manifestFiles: tt.manifestFiles,
			}
			err := m.validateNoSubDirectories()
			tt.assertErr(t, err)
		})
	}
}

func Test_manifests_validateOneObjectPerFile(t *testing.T) {
	tests := []struct {
		name          string
		manifestFiles []manifestFile
		assertErr     require.ErrorAssertionFunc
	}{
		{
			name: "one object per file is valid",
			manifestFiles: []manifestFile{
				{filename: "manifest1.yaml", objects: make([]client.Object, 1)},
				{filename: "manifest2.yaml", objects: make([]client.Object, 1)},
			},
			assertErr: require.NoError,
		},
		{
			name: "manifests with multiple objects are invalid",
			manifestFiles: []manifestFile{
				{filename: "manifest1.yaml", objects: make([]client.Object, 1)},
				{filename: "manifest2.yaml", objects: make([]client.Object, 1)},
				{filename: "manifest3.yaml", objects: make([]client.Object, 2)},
				{filename: "manifest4.yaml", objects: make([]client.Object, 3)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "manifest files must contain exactly one object")
				require.ErrorContains(t, err, "manifest3.yaml")
				require.ErrorContains(t, err, "manifest4.yaml")
			},
		},
		{
			name: "manifests with no objects are invalid",
			manifestFiles: []manifestFile{
				{filename: "manifest1.yaml", objects: make([]client.Object, 1)},
				{filename: "manifest2.yaml", objects: make([]client.Object, 1)},
				{filename: "manifest3.yaml", objects: make([]client.Object, 0)},
				{filename: "manifest4.yaml", objects: make([]client.Object, 0)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "manifest files must contain exactly one object")
				require.ErrorContains(t, err, "manifest3.yaml")
				require.ErrorContains(t, err, "manifest4.yaml")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manifests{
				manifestFiles: tt.manifestFiles,
			}
			err := m.validateOneObjectPerFile()
			tt.assertErr(t, err)
		})
	}
}

func Test_manifests_validateExactlyOneCSV(t *testing.T) {
	other := makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "Other"})
	csv := makeObject(v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.ClusterServiceVersionKind))

	tests := []struct {
		name          string
		manifestFiles []manifestFile
		assertErr     require.ErrorAssertionFunc
	}{
		{
			name: "exactly one csv among all files is valid",
			manifestFiles: []manifestFile{
				{filename: "manifest1.yaml", objects: []client.Object{other}},
				{filename: "manifest2.yaml", objects: []client.Object{other}},
				{filename: "csv1.yaml", objects: []client.Object{csv}},
			},
			assertErr: require.NoError,
		},
		{
			name: "zero csvs among all files is invalid",
			manifestFiles: []manifestFile{
				{filename: "manifest1.yaml", objects: []client.Object{other}},
				{filename: "manifest2.yaml", objects: []client.Object{other}},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "exactly one ClusterServiceVersion object is required, found 0")
			},
		},
		{
			// If there are zero manifests, there isn't a CSV.
			name:          "zero manifest files is invalid",
			manifestFiles: nil,
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "exactly one ClusterServiceVersion object is required, found 0")
			},
		},
		{
			name: "multiple csvs among all files is invalid",
			manifestFiles: []manifestFile{
				{filename: "manifest1.yaml", objects: []client.Object{other}},
				{filename: "manifest2.yaml", objects: []client.Object{other}},
				{filename: "csv1.yaml", objects: []client.Object{csv}},
				{filename: "csv2.yaml", objects: []client.Object{csv}},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "exactly one ClusterServiceVersion object is required, found 2")
				require.ErrorContains(t, err, "csv1.yaml")
				require.ErrorContains(t, err, "csv2.yaml")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manifests{
				manifestFiles: tt.manifestFiles,
			}
			err := m.validateExactlyOneCSV()
			tt.assertErr(t, err)
		})
	}
}

func Test_manifests_validateSupportedKinds(t *testing.T) {
	tests := []struct {
		name          string
		manifestFiles []manifestFile
		assertErr     require.ErrorAssertionFunc
	}{
		{
			name:          "using only supported kinds is valid",
			manifestFiles: supportedManifestFiles(),
			assertErr:     require.NoError,
		},
		{
			name:          "using any unsupported kind is invalid",
			manifestFiles: append(supportedManifestFiles(), unsupportedManifestFiles()...),
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "found unsupported kinds")
				require.ErrorContains(t, err, "Unsupported1.yaml")
				require.ErrorContains(t, err, "Unsupported2.yaml")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manifests{
				manifestFiles: tt.manifestFiles,
			}
			err := m.validateSupportedKinds()
			tt.assertErr(t, err)
		})
	}
}

func Test_manifests_validate(t *testing.T) {
	tests := []struct {
		name          string
		manifestFiles []manifestFile
		assertErr     require.ErrorAssertionFunc
	}{
		{
			name:          "passes all validations",
			manifestFiles: supportedManifestFiles(),
			assertErr:     require.NoError,
		},
		{
			name: "validate collects suberrors",
			manifestFiles: []manifestFile{
				{filename: "subdir/service.yaml", objects: []client.Object{
					makeObject(schema.GroupVersionKind{Version: "v1", Kind: "Service"}),
				}},
				{filename: "no_objects.yaml", objects: nil},
				{filename: "unsupported.yaml", objects: []client.Object{
					makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "Unsupported"}),
				}},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "subdirectories not allowed")
				require.ErrorContains(t, err, "manifest files must contain exactly one object")
				require.ErrorContains(t, err, "exactly one ClusterServiceVersion object is required, found 0")
				require.ErrorContains(t, err, "found unsupported kinds")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := &manifests{
				manifestFiles: tt.manifestFiles,
			}
			err := m.validate()
			tt.assertErr(t, err)
		})
	}
}

func makeObject(gvk schema.GroupVersionKind) client.Object {
	obj, _ := supportedKindsScheme.New(gvk)
	if obj == nil {
		obj = &unstructured.Unstructured{}
	}
	obj.GetObjectKind().SetGroupVersionKind(gvk)
	return obj.(client.Object)
}

func unsupportedManifestFiles() []manifestFile {
	unsupported1 := makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "Unsupported1"})
	unsupported2 := makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "Unsupported2"})

	return []manifestFile{
		{filename: "Unsupported1.yaml", objects: []client.Object{unsupported1}},
		{filename: "Unsupported2.yaml", objects: []client.Object{unsupported2}},
	}
}

func supportedManifestFiles() []manifestFile {
	kinds := sets.List(supportedKinds)
	manifestFiles := make([]manifestFile, 0, len(kinds))
	for _, kind := range kinds {
		obj := makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: kind})
		manifestFiles = append(manifestFiles, manifestFile{filename: fmt.Sprintf("%s.yaml", kind), objects: []client.Object{obj}})
	}
	return manifestFiles
}
