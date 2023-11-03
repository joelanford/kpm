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

func TestPackage(t *testing.T) {
	p := testutil.GetSimplePackage(t)

	annotations, err := p.Annotations()
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"kpm.io/displayName":      "Foo Operator",
		"kpm.io/name":             "foo",
		"kpm.io/providerName":     "Test Data",
		"kpm.io/shortDescription": "This is the Foo Operator.",
		"flim":                    "flam",
	}, annotations)

	assert.Equal(t, v1.ArtifactTypePackage, p.ArtifactType())

	config := p.Config()
	assert.Equal(t, v1.MediaTypePackageConfig, config.MediaType())
	configReader, err := config.Data()
	require.NoError(t, err)
	configData, err := io.ReadAll(configReader)
	require.NoError(t, err)
	assert.Equal(t, `{"name":"foo","displayName":"Foo Operator","providerName":"Test Data","shortDescription":"This is the Foo Operator.","maintainers":[{"name":"John Doe","email":"johndoe@example.com"},{"name":"Jane Doe","email":"janedoe@example.com"}],"categories":["operator","test"]}`, string(configData))

	blobs := p.Blobs()
	require.Len(t, blobs, 1)
	icon := blobs[0]
	assert.Equal(t, "image/svg+xml", icon.MediaType())
	contentReader, err := icon.Data()
	require.NoError(t, err)
	iconData, err := io.ReadAll(contentReader)
	require.NoError(t, err)
	assert.Equal(t, []byte(testutil.IconData), iconData)

	assert.Len(t, p.SubArtifacts(), 2)
	assert.NoError(t, p.Validate())
}

func TestPackage_Validate(t *testing.T) {
	type testCase struct {
		name   string
		pFunc  func(p *v1.Package)
		expect require.ErrorAssertionFunc
	}
	for _, tc := range []testCase{
		{
			name:  "missing name",
			pFunc: func(p *v1.Package) { p.Name = "" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "name is required")
			},
		},
		{
			name:  "invalid name",
			pFunc: func(p *v1.Package) { p.Name = "foo.bar" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "name is invalid")
			},
		},
		{
			name:  "missing display name",
			pFunc: func(p *v1.Package) { p.DisplayName = "" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "displayName is required")
			},
		},
		{
			name:  "missing provider name",
			pFunc: func(p *v1.Package) { p.ProviderName = "" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "providerName is required")
			},
		},
		{
			name:  "missing short description",
			pFunc: func(p *v1.Package) { p.ShortDescription = "" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "shortDescription is required")
			},
		},
		{
			name:  "short description too long",
			pFunc: func(p *v1.Package) { p.ShortDescription = strings.Repeat("x", 200) },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "shortDescription is too long")
			},
		},
		{
			name:  "missing maintainer",
			pFunc: func(p *v1.Package) { p.Maintainers = nil },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "at least one maintainer is required")
			},
		},
		{
			name:  "missing maintainer email",
			pFunc: func(p *v1.Package) { p.Maintainers[0].Email = "" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "maintainer[0] email is required")
			},
		},
		{
			name:  "invalid category",
			pFunc: func(p *v1.Package) { p.Categories = []string{"foo.bar"} },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "category is invalid")
			},
		},
		{
			name:  "invalid extra annotations",
			pFunc: func(p *v1.Package) { p.ExtraAnnotations = map[string]string{"kpm.io/name": "baz"} },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "invalid extra annotations")
			},
		},
		{
			name:  "missing bundle",
			pFunc: func(p *v1.Package) { p.Bundles = nil },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "at least one bundle is required")
			},
		},
		{
			name:  "bundle name does not match package name",
			pFunc: func(p *v1.Package) { p.Bundles[0].Name = "bar" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, `bundle[0] is invalid: bundle name "bar" must match package name "foo"`)
			},
		},
		{
			name:  "invalid bundle",
			pFunc: func(p *v1.Package) { p.Bundles[0].Version = "1.0.0.0" },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, "bundle[0] is invalid: version is invalid")
			},
		},
		{
			name:  "duplicate bundle",
			pFunc: func(p *v1.Package) { p.Bundles = append(p.Bundles, p.Bundles[0]) },
			expect: func(t require.TestingT, err error, _ ...interface{}) {
				assert.ErrorContains(t, err, `duplicate bundle "foo-1.0.0-1"`)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			p := testutil.GetSimplePackage(t)
			tc.pFunc(&p)
			tc.expect(t, p.Validate())
		})
	}
}
