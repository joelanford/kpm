package v1_test

import (
	"path/filepath"
	"testing"

	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/testutil"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func TestBundleSpec(t *testing.T) {
	var spec v1.BundleSpec
	require.NoError(t, yaml.Unmarshal(testutil.TestdataFile(t, filepath.Join("registry-v1", "foo-1.0.0-1.bundlespec.yaml")), &spec))
	require.Equal(t, v1.BundleSpec{
		TypeMeta: metav1.TypeMeta{
			Kind:       "BundleSpec",
			APIVersion: "kpm.io/v1alpha1",
		},
		Type: "bundle",
		Bundle: &v1.BundleSource{
			BundleConfig: v1.BundleConfig{
				Name:      "foo",
				Version:   "1.0.0",
				Release:   "1",
				Provides:  []string{"package(foo=1.0.0)"},
				Requires:  []string{"package(bar)", "api(widgets.acme.io/v1alpha1)"},
				Conflicts: []string{"package(foo-legacy)"},
			},
			Source: v1.BundleSourceSource{
				Type: "file",
				File: &v1.BundleSourceFile{
					Path: "./foo-1.0.0-1/manifests/csv.yaml",
				},
				MediaType: "application/yaml",
			},
			Annotations: map[string]string{
				"foo": "bar",
			},
		},
	}, spec)
}
