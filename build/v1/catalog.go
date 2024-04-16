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

type catalogBuilder struct {
	RootFS fs.FS
	opts   buildOptions
}

func NewCatalogBuilder(rootfs fs.FS, opts ...BuildOption) ArtifactBuilder {
	b := &catalogBuilder{
		RootFS: rootfs,
		opts: buildOptions{
			SpecReader: v1.DefaultFBCSpec,
			Log:        func(string, ...interface{}) {},
		},
	}
	for _, opt := range opts {
		opt(&b.opts)
	}
	return b
}

func (b *catalogBuilder) BuildArtifact(_ context.Context) (oci.Artifact, error) {
	// Read the bundle spec into a byte slice for unmarshalling.
	catalogSpecData, err := io.ReadAll(b.opts.SpecReader)
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
		return b.buildFBCCatalog(catalogSpec)
	case "semver":
		return b.buildSemverCatalog(catalogSpec)
	}
	return nil, fmt.Errorf("unsupported bundle source type: %s", catalogSpec.Type)
}

func (b *catalogBuilder) buildFBCCatalog(spec v1.CatalogSpec) (*v1.DockerImage, error) {
	cacheDir, err := os.MkdirTemp("", "kpm-catalog-build-cache-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(cacheDir)
	c, err := cache.New(cacheDir)
	if err != nil {
		return nil, err
	}

	catalogFS, err := fs.Sub(b.RootFS, cmp.Or(spec.FBC.CatalogDir, "."))
	if err != nil {
		return nil, err
	}

	b.opts.Log("building FBC cache")
	if err := c.Build(context.Background(), catalogFS); err != nil {
		return nil, err
	}

	blobFS := newMultiFS()
	blobFS.mount("catalog", catalogFS)
	blobFS.mount("tmp/cache", os.DirFS(cacheDir))

	b.opts.Log("generating image layers")
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

func (b *catalogBuilder) buildSemverCatalog(_ v1.CatalogSpec) (*v1.DockerImage, error) {
	return nil, fmt.Errorf("semver catalog not yet implemented")
}
