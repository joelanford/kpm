package testutil

import (
	"path/filepath"
	"testing"

	v1 "github.com/joelanford/kpm/api/v1"
)

func GetSimplePackage(t *testing.T) v1.Package {
	b1 := v1.Bundle{
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
			Content:          TestdataFile(t, filepath.Join("registry-v1", "foo-1.0.0-1", "manifests", "csv.yaml")),
		},
		ExtraAnnotations: map[string]string{
			"foo": "bar",
		},
	}
	b2 := v1.Bundle{
		BundleConfig: v1.BundleConfig{
			Name:      "foo",
			Version:   "1.0.0",
			Release:   "2",
			Provides:  []string{"package(foo=1.0.0)"},
			Requires:  []string{"package(bar)", "api(widgets.acme.io/v1alpha1)"},
			Conflicts: []string{"package(foo-legacy)"},
		},
		BundleContent: v1.BundleContent{
			ContentMediaType: "application/yaml",
			Content:          TestdataFile(t, filepath.Join("registry-v1", "foo-1.0.0-2", "manifests", "csv.yaml")),
		},
		ExtraAnnotations: map[string]string{
			"fizz": "buzz",
		},
	}
	p := v1.Package{
		PackageConfig: v1.PackageConfig{
			Name:             "foo",
			DisplayName:      "Foo Operator",
			ProviderName:     "Test Data",
			ShortDescription: "This is the Foo Operator.",
			Maintainers:      []v1.Maintainer{{Name: "John Doe", Email: "johndoe@example.com"}, {Name: "Jane Doe", Email: "janedoe@example.com"}},
			Categories:       []string{"operator", "test"},
		},
		LongDescription: "# Foo Operator\n\nFoo Operator is a Kubernetes operator for Foo.",
		Icon:            &v1.Icon{MediaType: "image/svg+xml", Data: []byte(IconData)},
		Bundles:         []v1.Bundle{b1, b2},
		ExtraAnnotations: map[string]string{
			"flim": "flam",
		},
	}
	return p
}

const IconData = `<svg viewBox=".5 .5 3 4" fill="none" stroke="#20b2a" stroke-linecap="round"><path d="M1 4h-.001 V1h2v.001 M1 2.6 h1v.001"/></svg>`
