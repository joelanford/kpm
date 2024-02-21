package v1_test

import (
	"io"
	"path/filepath"
	"testing"

	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBundle(t *testing.T) {
	p := testutil.GetSimplePackage(t)
	b := p.SubArtifacts()[0].(*v1.Bundle)

	annotations, err := b.Annotations()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"kpm.io/name":    "foo",
		"kpm.io/version": "1.0.0",
		"kpm.io/release": "1",
		"foo":            "bar",
	}, annotations)

	assert.Equal(t, v1.ArtifactTypeBundle, b.ArtifactType())

	config := b.Config()
	assert.Equal(t, v1.MediaTypeBundleConfig, config.MediaType())
	configReader, err := config.Data()
	require.NoError(t, err)
	configData, err := io.ReadAll(configReader)
	require.NoError(t, err)
	assert.Equal(t, `{"name":"foo","version":"1.0.0","release":"1","provides":["package(foo=1.0.0)"],"requires":["package(bar)","api(widgets.acme.io/v1alpha1)"],"conflicts":["package(foo-legacy)"]}`, string(configData))

	blobs := b.Blobs()
	require.Len(t, blobs, 1)
	content := blobs[0]
	assert.Equal(t, "application/yaml", content.MediaType())
	contentReader, err := content.Data()
	require.NoError(t, err)
	contentData, err := io.ReadAll(contentReader)
	require.NoError(t, err)
	assert.Equal(t, testutil.TestdataFile(t, filepath.Join("registry-v1", "foo-1.0.0-1", "manifests", "csv.yaml")), contentData)

	assert.Nil(t, b.SubArtifacts())
	assert.Equal(t, "foo-1.0.0-1", b.Tag())
	assert.NoError(t, b.Validate())
}

func TestBundle_Validate(t *testing.T) {
	type testCase struct {
		name   string
		bFunc  func(*v1.Bundle)
		expect require.ErrorAssertionFunc
	}
	for _, tc := range []testCase{
		{
			name:  "missing name",
			bFunc: func(b *v1.Bundle) { b.Name = "" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "name is required")
			},
		},
		{
			name:  "invalid name",
			bFunc: func(b *v1.Bundle) { b.Name = "foo.bar" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "name is invalid")
			},
		},
		{
			name:  "missing version",
			bFunc: func(b *v1.Bundle) { b.Version = "" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "version is required")
			},
		},
		{
			name:  "invalid version",
			bFunc: func(b *v1.Bundle) { b.Version = "1.0.0.0" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "version is invalid")
			},
		},
		{
			name:  "missing release",
			bFunc: func(b *v1.Bundle) { b.Release = "" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "release is required")
			},
		},
		{
			name:  "invalid release",
			bFunc: func(b *v1.Bundle) { b.Release = "foo.bar" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "release is invalid")
			},
		},
		{
			name:  "invalid extra annotations",
			bFunc: func(b *v1.Bundle) { b.ExtraAnnotations = map[string]string{"kpm.io/name": "baz"} },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "invalid extra annotations")
			},
		},
	} {
		b := testutil.GetSimplePackage(t).SubArtifacts()[0].(*v1.Bundle)
		t.Run(tc.name, func(t *testing.T) {
			tc.bFunc(b)
			tc.expect(t, b.Validate())
		})
	}
}
