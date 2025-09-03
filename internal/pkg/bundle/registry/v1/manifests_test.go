package v1

import (
	"fmt"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	v1 "k8s.io/api/core/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/sets"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"

	"github.com/operator-framework/kpm/internal/pkg/bundle/registry/internal"
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
	other := makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "Other"}, "other-name", "")
	csv := makeObject(v1alpha1.SchemeGroupVersion.WithKind(v1alpha1.ClusterServiceVersionKind), "csv-name", "")

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

func Test_manifestFiles_validateOwnedCRDs(t *testing.T) {
	tests := []struct {
		name          string
		manifestFiles manifestFiles
		assertErr     require.ErrorAssertionFunc
	}{
		{
			name: "valid",
			manifestFiles: manifestFiles{
				NewPrecomputedFile[[]client.Object]("csv.yaml", nil, []client.Object{&v1alpha1.ClusterServiceVersion{
					ObjectMeta: metav1.ObjectMeta{Name: "example.v0.0.1"},
					Spec: v1alpha1.ClusterServiceVersionSpec{
						CustomResourceDefinitions: v1alpha1.CustomResourceDefinitions{
							Owned: []v1alpha1.CRDDescription{
								{Name: "bars.group.example.com", Version: "v1", Kind: "Bar"},
								{Name: "bars.group.example.com", Version: "v2", Kind: "Bar"},
								{Name: "foos.group.example.com", Version: "v1alpha1", Kind: "Foo"},
								{Name: "foos.group.example.com", Version: "v1alpha2", Kind: "Foo"},
							},
						},
					},
				}}),
				NewPrecomputedFile[[]client.Object]("foos.crd.yaml", nil, []client.Object{&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "foos.group.example.com"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Names: apiextensionsv1.CustomResourceDefinitionNames{
							Kind: "Foo",
						},
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{Name: "v1alpha1"}, {Name: "v1alpha2"},
						},
					},
				}}),
				NewPrecomputedFile[[]client.Object]("bars.crd.yaml", nil, []client.Object{&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "bars.group.example.com"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Names: apiextensionsv1.CustomResourceDefinitionNames{
							Kind: "Bar",
						},
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{Name: "v1"}, {Name: "v2"},
						},
					},
				}}),
			},
			assertErr: require.NoError,
		},
		{
			name: "missing CRD version in CRD manifest",
			manifestFiles: manifestFiles{
				NewPrecomputedFile[[]client.Object]("csv.yaml", nil, []client.Object{&v1alpha1.ClusterServiceVersion{
					ObjectMeta: metav1.ObjectMeta{Name: "example.v0.0.1"},
					Spec: v1alpha1.ClusterServiceVersionSpec{
						CustomResourceDefinitions: v1alpha1.CustomResourceDefinitions{
							Owned: []v1alpha1.CRDDescription{
								{Name: "bars.group.example.com", Version: "v1", Kind: "Bar"},
								{Name: "bars.group.example.com", Version: "v2", Kind: "Bar"},
								{Name: "foos.group.example.com", Version: "v1alpha1", Kind: "Foo"},
								{Name: "foos.group.example.com", Version: "v1alpha2", Kind: "Foo"},
							},
						},
					},
				}}),
				NewPrecomputedFile[[]client.Object]("foos.crd.yaml", nil, []client.Object{&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "foos.group.example.com"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Names: apiextensionsv1.CustomResourceDefinitionNames{
							Kind: "Foo",
						},
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{Name: "v1alpha1"}, {Name: "v1alpha2"},
						},
					},
				}}),
				NewPrecomputedFile[[]client.Object]("bars.crd.yaml", nil, []client.Object{&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "bars.group.example.com"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Names: apiextensionsv1.CustomResourceDefinitionNames{
							Kind: "Bar",
						},
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{Name: "v1"},
						},
					},
				}}),
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `CSV-owned CRD "bars.group.example.com", version "v2" not found in manifests`)
			},
		},
		{
			name: "missing CRDs",
			manifestFiles: manifestFiles{
				NewPrecomputedFile[[]client.Object]("csv.yaml", nil, []client.Object{&v1alpha1.ClusterServiceVersion{
					ObjectMeta: metav1.ObjectMeta{Name: "example.v0.0.1"},
					Spec: v1alpha1.ClusterServiceVersionSpec{
						CustomResourceDefinitions: v1alpha1.CustomResourceDefinitions{
							Owned: []v1alpha1.CRDDescription{
								{Name: "bars.group.example.com", Version: "v1", Kind: "Bar"},
								{Name: "bars.group.example.com", Version: "v2", Kind: "Bar"},
								{Name: "foos.group.example.com", Version: "v1alpha1", Kind: "Foo"},
								{Name: "foos.group.example.com", Version: "v1alpha2", Kind: "Foo"},
							},
						},
					},
				}}),
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `CSV-owned CRD "bars.group.example.com", version "v1" not found in manifests`)
				require.ErrorContains(t, err, `CSV-owned CRD "bars.group.example.com", version "v2" not found in manifests`)
				require.ErrorContains(t, err, `CSV-owned CRD "foos.group.example.com", version "v1alpha1" not found in manifests`)
				require.ErrorContains(t, err, `CSV-owned CRD "foos.group.example.com", version "v1alpha2" not found in manifests`)
			},
		},
		{
			name: "CSV missing owned CRD",
			manifestFiles: manifestFiles{
				NewPrecomputedFile[[]client.Object]("csv.yaml", nil, []client.Object{&v1alpha1.ClusterServiceVersion{
					ObjectMeta: metav1.ObjectMeta{Name: "example.v0.0.1"},
				}}),
				NewPrecomputedFile[[]client.Object]("foos.crd.yaml", nil, []client.Object{&apiextensionsv1.CustomResourceDefinition{
					ObjectMeta: metav1.ObjectMeta{Name: "foos.group.example.com"},
					Spec: apiextensionsv1.CustomResourceDefinitionSpec{
						Names: apiextensionsv1.CustomResourceDefinitionNames{
							Kind: "Foo",
						},
						Versions: []apiextensionsv1.CustomResourceDefinitionVersion{
							{Name: "v1alpha1"}, {Name: "v1alpha2"},
						},
					},
				}}),
			},
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, `CRD "foos.group.example.com", version "v1alpha1" not owned by CSV`)
				require.ErrorContains(t, err, `CRD "foos.group.example.com", version "v1alpha2" not owned by CSV`)
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifestFiles.validateOwnedAPIs()
			tt.assertErr(t, err)
		})
	}
}

func Test_manifestFiles_validateUniqueGroupKindName(t *testing.T) {
	tests := []struct {
		name          string
		manifestFiles manifestFiles
		assertErr     require.ErrorAssertionFunc
	}{
		{
			name:          "valid",
			manifestFiles: supportedManifestFiles(),
			assertErr:     require.NoError,
		},
		{
			name: "valid: duplicate GK, different names",
			manifestFiles: manifestFiles{
				NewPrecomputedFile[[]client.Object]("manifest1.yaml", nil, []client.Object{
					makeObject(appsv1.SchemeGroupVersion.WithKind("Deployment"), "name1", ""),
				}),
				NewPrecomputedFile[[]client.Object]("manifest2.yaml", nil, []client.Object{
					makeObject(appsv1.SchemeGroupVersion.WithKind("Deployment"), "name2", ""),
				}),
			},
			assertErr: require.NoError,
		},
		{
			name: "valid: duplicate names, different GKs",
			manifestFiles: manifestFiles{
				NewPrecomputedFile[[]client.Object]("manifest1.yaml", nil, []client.Object{
					makeObject(appsv1.SchemeGroupVersion.WithKind("Deployment"), "name", ""),
				}),
				NewPrecomputedFile[[]client.Object]("manifest2.yaml", nil, []client.Object{
					makeObject(appsv1.SchemeGroupVersion.WithKind("ReplicaSet"), "name", ""),
				}),
			},
			assertErr: require.NoError,
		},
		{
			name: "invalid: duplicate GKN",
			manifestFiles: manifestFiles{
				NewPrecomputedFile[[]client.Object]("manifest1.yaml", nil, []client.Object{
					makeObject(appsv1.SchemeGroupVersion.WithKind("Deployment"), "dep", ""),
				}),
				NewPrecomputedFile[[]client.Object]("manifest2.yaml", nil, []client.Object{
					makeObject(appsv1.SchemeGroupVersion.WithKind("Deployment"), "dep", ""),
				}),
			},
			assertErr: require.Error,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.manifestFiles.validateUniqueGroupKindName()
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
				NewPrecomputedFile[[]client.Object]("subdir/service.yaml", nil, []client.Object{makeObject(schema.GroupVersionKind{Version: "v1", Kind: "Service"}, "svc", "")}),
				NewPrecomputedFile[[]client.Object]("no_objects.yaml", nil, nil),
				NewPrecomputedFile[[]client.Object]("unsupported.yaml", nil, []client.Object{makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "Unsupported"}, "u", "")}),
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

func makeObject(gvk schema.GroupVersionKind, name string, namespace string) client.Object {
	obj, _ := internal.SupportedKindsScheme.New(gvk)
	if obj == nil {
		obj = &unstructured.Unstructured{}
	}
	cObj := obj.(client.Object)
	cObj.GetObjectKind().SetGroupVersionKind(gvk)
	cObj.SetName(name)
	cObj.SetNamespace(namespace)
	return cObj
}

func unsupportedManifestFiles() []File[[]client.Object] {
	unsupported1 := makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "Unsupported1"}, "u1", "")
	unsupported2 := makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: "Unsupported2"}, "u2", "")

	return []File[[]client.Object]{
		NewPrecomputedFile[[]client.Object]("Unsupported1.yaml", nil, []client.Object{unsupported1}),
		NewPrecomputedFile[[]client.Object]("Unsupported2.yaml", nil, []client.Object{unsupported2}),
	}
}

func supportedManifestFiles() []File[[]client.Object] {
	kinds := sets.List(internal.SupportedKinds)
	mf := make([]File[[]client.Object], 0, len(kinds))
	for i, kind := range kinds {
		obj := makeObject(schema.GroupVersionKind{Group: "example.com", Version: "v1alpha1", Kind: kind}, fmt.Sprintf("obj%d", i), "")
		mf = append(mf, NewPrecomputedFile(fmt.Sprintf("%s.yaml", kind), nil, []client.Object{obj}))
	}
	return mf
}
