package v1

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"

	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/oci"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
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
			SpecReader: v1.DefaultRegistryV1Spec,
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
		//return b.buildFBCBundle(catalogSpec)
	}
	return nil, fmt.Errorf("unsupported bundle source type: %s", bundleSpec.Type)
}

func (b *bundleBuilder) buildRegistryV1(spec v1.RegistryV1Source) (*v1.DockerImage, error) {
	manifestsFS, err := fs.Sub(b.RootFS, cmp.Or(spec.ManifestsDir, "manifests"))
	if err != nil {
		return nil, err
	}

	metadataFS, err := fs.Sub(b.RootFS, cmp.Or(spec.MetadataDir, "metadata"))
	if err != nil {
		return nil, err
	}

	version, err := getRegistryBundleVersion(manifestsFS)
	if err != nil {
		return nil, err
	}

	annotations, err := readAnnotations(metadataFS)
	if err != nil {
		return nil, err
	}

	blobFS := newMultiFS()
	blobFS.mount("manifests", manifestsFS)
	blobFS.mount("metadata", metadataFS)

	b.opts.Log("generating image layers")
	blobData, err := getBlobData(blobFS)
	if err != nil {
		return nil, err
	}

	configData, err := getConfigData(annotations, blobData)
	if err != nil {
		return nil, err
	}

	return v1.NewDockerImage(version, configData, blobData, annotations), nil
}

func getRegistryBundleVersion(manifestsFS fs.FS) (string, error) {
	manifestFiles, err := fs.ReadDir(manifestsFS, ".")
	if err != nil {
		return "", err
	}

	var (
		csvData []byte
		errs    []error
	)

	for _, manifestFile := range manifestFiles {
		if manifestFile.IsDir() {
			continue
		}
		manifestFileData, err := fs.ReadFile(manifestsFS, manifestFile.Name())
		if err != nil {
			errs = append(errs, err)
			continue
		}
		var m metav1.PartialObjectMetadata
		if err := yaml.Unmarshal(manifestFileData, &m); err != nil {
			errs = append(errs, err)
			continue
		}
		if m.Kind == "ClusterServiceVersion" {
			csvData = manifestFileData
			break
		}
	}

	if len(errs) > 0 {
		return "", errors.Join(errs...)
	}
	if csvData == nil {
		return "", errors.New("no ClusterServiceVersion found in manifests")
	}

	var csv unstructured.Unstructured
	if err := yaml.Unmarshal(csvData, &csv); err != nil {
		return "", err
	}
	version, found, err := unstructured.NestedString(csv.Object, "spec", "version")
	if err != nil || !found {
		// Fall back to the name if the version is not set
		version = csv.GetName()
	}

	return version, nil
}

func readAnnotations(metadataFS fs.FS) (map[string]string, error) {
	annotationsFile, err := fs.ReadFile(metadataFS, "annotations.yaml")
	if err != nil {
		return nil, err
	}
	var annotations struct {
		Annotations map[string]string `yaml:"annotations"`
	}
	if err := yaml.Unmarshal(annotationsFile, &annotations); err != nil {
		return nil, err
	}

	annotations.Annotations["operators.operatorframework.io.bundle.manifests.v1"] = "manifests/"
	annotations.Annotations["operators.operatorframework.io.bundle.metadata.v1"] = "metadata/"
	return annotations.Annotations, nil
}
