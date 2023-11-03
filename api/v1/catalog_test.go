package v1_test

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/testutil"
)

func TestCatalog(t *testing.T) {
	c := testutil.GetSimpleCatalog(t)

	annotations, err := c.Annotations()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"kpm.io/displayName":      "Test Catalog",
		"kpm.io/providerName":     "Test Provider",
		"kpm.io/shortDescription": "This is a test catalog.",
		"hop":                     "skip",
	}, annotations)

	assert.Equal(t, v1.ArtifactTypeCatalog, c.ArtifactType())

	config := c.Config()
	assert.Equal(t, v1.MediaTypeCatalogConfig, config.MediaType())
	configReader, err := config.Data()
	require.NoError(t, err)
	configData, err := io.ReadAll(configReader)
	require.NoError(t, err)
	assert.Equal(t, `{"displayName":"Test Catalog","providerName":"Test Provider","shortDescription":"This is a test catalog."}`, string(configData))

	blobs := c.Blobs()
	require.Len(t, blobs, 1)
	icon := blobs[0]
	assert.Equal(t, "image/svg+xml", icon.MediaType())
	contentReader, err := icon.Data()
	require.NoError(t, err)
	iconData, err := io.ReadAll(contentReader)
	require.NoError(t, err)
	assert.Equal(t, []byte(testutil.IconData), iconData)

	assert.Len(t, c.SubArtifacts(), 1)
	assert.NoError(t, c.Validate())
}

func TestCatalog_Validate(t *testing.T) {
	type testCase struct {
		name   string
		cFunc  func(c *v1.Catalog)
		expect require.ErrorAssertionFunc
	}
	for _, tc := range []testCase{
		{
			name:  "missing display name",
			cFunc: func(c *v1.Catalog) { c.DisplayName = "" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "displayName is required")
			},
		},
		{
			name:  "missing provider name",
			cFunc: func(c *v1.Catalog) { c.ProviderName = "" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "providerName is required")
			},
		},
		{
			name:  "missing short description",
			cFunc: func(c *v1.Catalog) { c.ShortDescription = "" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "shortDescription is required")
			},
		},
		{
			name:  "short description too long",
			cFunc: func(c *v1.Catalog) { c.ShortDescription = strings.Repeat("x", 200) },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "shortDescription is too long")
			},
		},
		{
			name:  "invalid extra annotations",
			cFunc: func(c *v1.Catalog) { c.ExtraAnnotations = map[string]string{"kpm.io/displayName": "baz"} },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "invalid extra annotations")
			},
		},
		{
			name:  "missing package",
			cFunc: func(c *v1.Catalog) { c.Packages = nil },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "at least one package is required")
			},
		},
		{
			name:  "invalid package",
			cFunc: func(c *v1.Catalog) { c.Packages[0].Name = "foo.bar" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "package[0] is invalid: name is invalid")
			},
		},
		{
			name:  "duplicate package",
			cFunc: func(c *v1.Catalog) { c.Packages = append(c.Packages, c.Packages[0]) },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, `duplicate package "foo"`)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := testutil.GetSimpleCatalog(t)
			tc.cFunc(&c)
			tc.expect(t, c.Validate())
		})
	}
}
