package v1

import (
	"fmt"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"

	"github.com/joelanford/kpm/internal/pkg/bundle/registry/internal"
)

func Test_manifestsFSLoader_loadFiles(t *testing.T) {
	tests := []struct {
		name      string
		fsys      fs.FS
		expected  manifestFiles
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "loads files successfully",
			fsys: fstest.MapFS{
				"manifest.yaml": &fstest.MapFile{Data: []byte(`---
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
---
apiVersion: v1
kind: ConfigMap
---
apiVersion: example.com/v1alpha1
kind: SomethingElse
`)},
			},
			expected: []File[[]client.Object]{
				NewPrecomputedFile("manifest.yaml", []byte(`---
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
---
apiVersion: v1
kind: ConfigMap
---
apiVersion: example.com/v1alpha1
kind: SomethingElse
`), []client.Object{
					&v1alpha1.ClusterServiceVersion{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ClusterServiceVersion",
							APIVersion: "operators.coreos.com/v1alpha1",
						},
					},
					&v1.ConfigMap{
						TypeMeta: metav1.TypeMeta{
							Kind:       "ConfigMap",
							APIVersion: "v1",
						},
					},
					&unstructured.Unstructured{
						Object: map[string]interface{}{
							"apiVersion": "example.com/v1alpha1",
							"kind":       "SomethingElse",
						},
					},
				})},
			assertErr: require.NoError,
		},
		{
			name: "fails due to invalid yaml",
			fsys: fstest.MapFS{
				"invalid.yaml": &fstest.MapFile{Data: []byte(`}`)},
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "error parsing invalid.yaml")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := manifestsFSLoader{fsys: tt.fsys}
			files, err := m.loadFiles()
			tt.assertErr(t, err)
			require.Equal(t, tt.expected, files)
		})
	}
}

func Test_manifestFiles_validateNoSubDirectories(t *testing.T) {
	tests := []struct {
		name          string
		manifestFiles manifestFiles
		assertErr     require.ErrorAssertionFunc
	}{
		{
			name: "no sub directories, no error",
			manifestFiles: []File[[]client.Object]{
				NewPrecomputedFile[[]client.Object]("manifest1.yaml", nil, nil),
				NewPrecomputedFile[[]client.Object]("manifest2.yaml", nil, nil),
			},
			assertErr: require.NoError,
		},
		{
			name: "sub directory presence causes error",
			manifestFiles: []File[[]client.Object]{
				NewPrecomputedFile[[]client.Object]("manifest1.yaml", nil, nil),
				NewPrecomputedFile[[]client.Object]("manifest2.yaml", nil, nil),
				NewPrecomputedFile[[]client.Object]("subdir1/manifest10.yaml", nil, nil),
				NewPrecomputedFile[[]client.Object]("subdir1/manifest11.yaml", nil, nil),
				NewPrecomputedFile[[]client.Object]("subdir2/manifest20.yaml", nil, nil),
				NewPrecomputedFile[[]client.Object]("subdir2/manifest21.yaml", nil, nil),
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
			err := tt.manifestFiles.validateNoSubDirectories()
			tt.assertErr(t, err)
		})
	}
}

func Test_manifestFiles_validateOneObjectPerFile(t *testing.T) {
	tests := []struct {
		name          string
		manifestFiles manifestFiles
		assertErr     require.ErrorAssertionFunc
	}{
		{
			name: "one object per file is valid",
			manifestFiles: []File[[]client.Object]{
				NewPrecomputedFile[[]client.Object]("manifest1.yaml", nil, make([]client.Object, 1)),
				NewPrecomputedFile[[]client.Object]("manifest2.yaml", nil, make([]client.Object, 1)),
			},
			assertErr: require.NoError,
		},
		{
			name: "manifests with multiple objects are invalid",
			manifestFiles: []File[[]client.Object]{
				NewPrecomputedFile[[]client.Object]("manifest1.yaml", nil, make([]client.Object, 1)),
				NewPrecomputedFile[[]client.Object]("manifest2.yaml", nil, make([]client.Object, 1)),
				NewPrecomputedFile[[]client.Object]("manifest3.yaml", nil, make([]client.Object, 2)),
				NewPrecomputedFile[[]client.Object]("manifest4.yaml", nil, make([]client.Object, 3)),
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "manifest files must contain exactly one object")
				require.ErrorContains(t, err, "manifest3.yaml")
				require.ErrorContains(t, err, "manifest4.yaml")
			},
		},
		{
			name: "manifests with no objects are invalid",
			manifestFiles: []File[[]client.Object]{
				NewPrecomputedFile[[]client.Object]("manifest1.yaml", nil, make([]client.Object, 1)),
				NewPrecomputedFile[[]client.Object]("manifest2.yaml", nil, make([]client.Object, 1)),
				NewPrecomputedFile[[]client.Object]("manifest3.yaml", nil, make([]client.Object, 0)),
				NewPrecomputedFile[[]client.Object]("manifest4.yaml", nil, make([]client.Object, 0)),
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
			err := tt.manifestFiles.validateOneObjectPerFile()
			tt.assertErr(t, err)
		})
	}
}

func Test_manifestFiles_validateExactlyOneCSV(t *testing.T) {
	other := makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "Other"})
	csv := makeObject(v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.ClusterServiceVersionKind))

	tests := []struct {
		name          string
		manifestFiles manifestFiles
		assertErr     require.ErrorAssertionFunc
	}{
		{
			name: "exactly one csv among all files is valid",
			manifestFiles: []File[[]client.Object]{
				NewPrecomputedFile[[]client.Object]("manifest1.yaml", nil, []client.Object{other}),
				NewPrecomputedFile[[]client.Object]("manifest2.yaml", nil, []client.Object{other}),
				NewPrecomputedFile[[]client.Object]("csv1.yaml", nil, []client.Object{csv}),
			},
			assertErr: require.NoError,
		},
		{
			name: "zero csvs among all files is invalid",
			manifestFiles: []File[[]client.Object]{
				NewPrecomputedFile[[]client.Object]("manifest1.yaml", nil, []client.Object{other}),
				NewPrecomputedFile[[]client.Object]("manifest2.yaml", nil, []client.Object{other}),
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
			manifestFiles: []File[[]client.Object]{
				NewPrecomputedFile[[]client.Object]("manifest1.yaml", nil, []client.Object{other}),
				NewPrecomputedFile[[]client.Object]("manifest2.yaml", nil, []client.Object{other}),
				NewPrecomputedFile[[]client.Object]("csv1.yaml", nil, []client.Object{csv}),
				NewPrecomputedFile[[]client.Object]("csv2.yaml", nil, []client.Object{csv}),
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
			err := tt.manifestFiles.validateExactlyOneCSV()
			tt.assertErr(t, err)
		})
	}
}

func Test_manifestFiles_validateSupportedKinds(t *testing.T) {
	tests := []struct {
		name          string
		manifestFiles manifestFiles
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
			err := tt.manifestFiles.validateSupportedKinds()
			tt.assertErr(t, err)
		})
	}
}

func Test_manifestFiles_validate(t *testing.T) {
	tests := []struct {
		name          string
		manifestFiles manifestFiles
		assertErr     require.ErrorAssertionFunc
	}{
		{
			name:          "passes all validations",
			manifestFiles: supportedManifestFiles(),
			assertErr:     require.NoError,
		},
		{
			name: "validate collects suberrors",
			manifestFiles: []File[[]client.Object]{
				NewPrecomputedFile[[]client.Object]("subdir/service.yaml", nil, []client.Object{makeObject(schema.GroupVersionKind{Version: "v1", Kind: "Service"})}),
				NewPrecomputedFile[[]client.Object]("no_objects.yaml", nil, nil),
				NewPrecomputedFile[[]client.Object]("unsupported.yaml", nil, []client.Object{makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "Unsupported"})}),
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
			err := tt.manifestFiles.validate()
			tt.assertErr(t, err)
		})
	}
}

func makeObject(gvk schema.GroupVersionKind) client.Object {
	obj, _ := internal.SupportedKindsScheme.New(gvk)
	if obj == nil {
		obj = &unstructured.Unstructured{}
	}
	obj.GetObjectKind().SetGroupVersionKind(gvk)
	return obj.(client.Object)
}

func unsupportedManifestFiles() []File[[]client.Object] {
	unsupported1 := makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "Unsupported1"})
	unsupported2 := makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "Unsupported2"})

	return []File[[]client.Object]{
		NewPrecomputedFile[[]client.Object]("Unsupported1.yaml", nil, []client.Object{unsupported1}),
		NewPrecomputedFile[[]client.Object]("Unsupported2.yaml", nil, []client.Object{unsupported2}),
	}
}

func supportedManifestFiles() []File[[]client.Object] {
	kinds := sets.List(internal.SupportedKinds)
	manifestFiles := make([]File[[]client.Object], 0, len(kinds))
	for _, kind := range kinds {
		obj := makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: kind})
		manifestFiles = append(manifestFiles, NewPrecomputedFile(fmt.Sprintf("%s.yaml", kind), nil, []client.Object{obj}))
	}
	return manifestFiles
}
