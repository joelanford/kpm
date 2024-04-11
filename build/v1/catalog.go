package v1

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"

	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/oci"
	"sigs.k8s.io/yaml"

	"github.com/operator-framework/operator-registry/pkg/cache"
	"github.com/operator-framework/operator-registry/pkg/containertools"
)

func Catalog(catalogSpecReader io.Reader, workingFS fs.FS) (oci.Artifact, error) {
	// Read the bundle spec into a byte slice for unmarshalling.
	catalogSpecData, err := io.ReadAll(catalogSpecReader)
	if err != nil {
		return nil, fmt.Errorf("read bundle spec: %w", err)
	}

	// Unmarshal the bundle spec.
	var catalogSpec v1.CatalogSpec
	if err := yaml.Unmarshal(catalogSpecData, &catalogSpec); err != nil {
		return nil, fmt.Errorf("unmarshal bundle spec: %w", err)
	}

	switch catalogSpec.Type {
	case "fbc":
		return buildFBCCatalog(catalogSpec, workingFS)
	case "semver":
		return buildSemverCatalog(catalogSpec, workingFS)
	}
	return nil, fmt.Errorf("unsupported bundle source type: %s", catalogSpec.Type)
}

func buildFBCCatalog(spec v1.CatalogSpec, workingFS fs.FS) (*v1.DockerImage, error) {
	cacheDir, err := os.MkdirTemp("", "kpm-catalog-build-cache-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(cacheDir)
	c, err := cache.New(cacheDir)
	if err != nil {
		return nil, err
	}

	catalogFS, err := fs.Sub(workingFS, cmp.Or(spec.FBC.CatalogDir, "."))
	if err != nil {
		return nil, err
	}

	if err := c.Build(context.Background(), catalogFS); err != nil {
		return nil, err
	}

	blobFS := newMultiFS()
	blobFS.mount("catalog", catalogFS)
	blobFS.mount("tmp/cache", os.DirFS(cacheDir))

	blobData, err := getBlobData(blobFS)
	if err != nil {
		return nil, err
	}

	annotations := spec.ExtraAnnotations
	if annotations == nil {
		annotations = make(map[string]string, 5)
	}
	annotations[containertools.ConfigsLocationLabel] = "/catalog"
	annotations["operators.operatorframework.io.index.cache.v1"] = "/tmp/cache"

	if spec.DisplayName != "" {
		annotations["operators.operatorframework.io.displayName.v1"] = spec.DisplayName
	}
	if spec.Description != "" {
		annotations["operators.operatorframework.io.description.v1"] = spec.Description
	}
	if spec.Publisher != "" {
		annotations["operators.operatorframework.io.publisher.v1"] = spec.Publisher
	}

	configData, err := getConfigData(annotations, blobData)
	if err != nil {
		return nil, err
	}

	return v1.NewDockerImage(cmp.Or(spec.Tag, "latest"), configData, blobData, annotations), nil
}

func buildSemverCatalog(spec v1.CatalogSpec, workingFS fs.FS) (*v1.DockerImage, error) {
	return nil, fmt.Errorf("semver catalog not yet implemented")
}
