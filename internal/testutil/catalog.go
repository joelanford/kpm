package testutil

import (
	"testing"

	v1 "github.com/joelanford/kpm/api/v1"
)

func GetSimpleCatalog(t *testing.T) v1.Catalog {
	return v1.Catalog{
		CatalogConfig: v1.CatalogConfig{
			DisplayName:      "Test Catalog",
			ProviderName:     "Test Provider",
			ShortDescription: "This is a test catalog.",
		},
		Icon: &v1.Icon{MediaType: "image/svg+xml", Data: []byte(IconData)},
		Packages: []v1.Package{
			GetSimplePackage(t),
		},
		ExtraAnnotations: map[string]string{
			"hop": "skip",
		},
	}
}
