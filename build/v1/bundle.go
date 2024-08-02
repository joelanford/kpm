package v1

import (
	"cmp"
	"context"
	"fmt"
	"io"
	"io/fs"
	"strings"

	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/fsutil"
	"github.com/joelanford/kpm/internal/registryv1"
	"github.com/joelanford/kpm/oci"
	"sigs.k8s.io/yaml"
)

type bundleBuilder struct {
	RootFS fs.FS
	opts   buildOptions
}

func NewBundleBuilder(rootfs fs.FS, opts ...BuildOption) ArtifactBuilder {
	b := &bundleBuilder{
		RootFS: rootfs,
		opts: buildOptions{
			SpecReader: strings.NewReader(v1.DefaultRegistryV1Spec),
			Log:        func(string, ...interface{}) {},
		},
	}
	for _, opt := range opts {
		opt(&b.opts)
	}
	return b
}

func (b *bundleBuilder) BuildArtifact(_ context.Context) (oci.Artifact, error) {
	// Read the bundle spec into a byte slice for unmarshalling.
	bundleSpecData, err := io.ReadAll(b.opts.SpecReader)
	if err != nil {
		return nil, fmt.Errorf("read bundle spec: %w", err)
	}

	// Unmarshal the bundle spec.
	var bundleSpec v1.BundleSpec
	if err := yaml.Unmarshal(bundleSpecData, &bundleSpec); err != nil {
		return nil, fmt.Errorf("unmarshal bundle spec: %w", err)
	}

	switch bundleSpec.Type {
	case "registryV1":
		return b.buildRegistryV1(*bundleSpec.RegistryV1)
	}
	return nil, fmt.Errorf("unsupported bundle source type: %s", bundleSpec.Type)
}

func (b *bundleBuilder) buildRegistryV1(spec v1.RegistryV1Source) (*v1.OCIManifest, error) {
	manifestsFS, err := fs.Sub(b.RootFS, cmp.Or(spec.ManifestsDir, "manifests"))
	if err != nil {
		return nil, err
	}

	metadataFS, err := fs.Sub(b.RootFS, cmp.Or(spec.MetadataDir, "metadata"))
	if err != nil {
		return nil, err
	}

	version, err := registryv1.GetVersion(manifestsFS)
	if err != nil {
		return nil, err
	}

	annotations, err := registryv1.GetAnnotations(metadataFS)
	if err != nil {
		return nil, err
	}

	pkgName, pkgNameFound := annotations["operators.operatorframework.io.bundle.package.v1"]
	if !pkgNameFound {
		return nil, fmt.Errorf("registry+v1 bundle is missing required package name annotation")
	}

	release, foundRelease := annotations["operators.operatorframework.io.bundle.release.v1"]
	if !foundRelease {
		release = "0"
	}
	manifestsFS, _ = fsutil.Prefix(manifestsFS, "manifests")
	metadataFS, _ = fsutil.Prefix(metadataFS, "metadata")

	b.opts.Log("generating image layers")
	blobData, err := getBlobData(manifestsFS, metadataFS)
	if err != nil {
		return nil, err
	}

	configData, err := getConfigData(annotations, blobData)
	if err != nil {
		return nil, err
	}

	tag := fmt.Sprintf("%s-v%s-%s", pkgName, version, release)
	return v1.NewOCIManifest(tag, configData, blobData, annotations), nil
}
