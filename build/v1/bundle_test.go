package v1_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	v1 "github.com/joelanford/kpm/api/v1"
	buildv1 "github.com/joelanford/kpm/build/v1"
	"github.com/joelanford/kpm/internal/testutil"
)

func TestBundle(t *testing.T) {
	specFile := testutil.TestdataFile(t, filepath.Join("registry-v1", "foo-1.0.0-1.bundlespec.yaml"))
	wd := filepath.Join("..", "..", "internal", "testutil", "testdata", "registry-v1")
	bundle, err := buildv1.Bundle(bytes.NewReader(specFile), os.DirFS(wd))
	require.NoError(t, err)
	require.Equal(t, &v1.Bundle{
		BundleConfig: v1.BundleConfig{
			Name:      "foo",
			Version:   "1.0.0",
			Release:   "1",
			Provides:  []string{"package(foo=1.0.0)"},
			Requires:  []string{"package(bar)", "api(widgets.acme.io/v1alpha1)"},
			Conflicts: []string{"package(foo-legacy)"},
		},
		BundleContent: v1.BundleContent{
			ContentMediaType: "application/yaml",
			Content:          testutil.TestdataFile(t, filepath.Join("registry-v1", "foo-1.0.0-1", "manifests", "csv.yaml")),
		},
		ExtraAnnotations: map[string]string{
			"foo": "bar",
		},
	}, bundle)
}
