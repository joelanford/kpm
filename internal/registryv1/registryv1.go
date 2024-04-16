package registryv1

import (
	"errors"
	"io/fs"

	"github.com/Masterminds/semver/v3"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func GetVersion(manifestsFS fs.FS) (*semver.Version, error) {
	manifestFiles, err := fs.ReadDir(manifestsFS, ".")
	if err != nil {
		return nil, err
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
		return nil, errors.Join(errs...)
	}
	if csvData == nil {
		return nil, errors.New("no ClusterServiceVersion found in manifests")
	}

	var csv unstructured.Unstructured
	if err := yaml.Unmarshal(csvData, &csv); err != nil {
		return nil, err
	}
	versionStr, found, err := unstructured.NestedString(csv.Object, "spec", "version")
	if err != nil || !found {
		return nil, errors.New("no version found in ClusterServiceVersion")
	}
	return semver.NewVersion(versionStr)
}

func GetAnnotations(metadataFS fs.FS) (map[string]string, error) {
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
