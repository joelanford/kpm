package v1

import (
	"bytes"
	"cmp"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io/fs"
	"path/filepath"
	"strings"
	"testing/fstest"

	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/tar"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"
)

func RegistryBundle(spec v1.RegistryV1Source, rootFS fs.FS) (*v1.RegistryBundle, error) {
	manifestsFS, err := fs.Sub(rootFS, cmp.Or(spec.ManifestsDir, "manifests"))
	if err != nil {
		return nil, err
	}

	metadataFS, err := fs.Sub(rootFS, cmp.Or(spec.MetadataDir, "metadata"))
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

	blobData, err := getBlobData(manifestsFS, metadataFS)
	if err != nil {
		return nil, err
	}

	configData, err := getConfigData(annotations, blobData)
	if err != nil {
		return nil, err
	}

	return v1.NewRegistryBundle(version, configData, blobData, annotations), nil
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

func getConfigData(annotations map[string]string, blobData []byte) ([]byte, error) {
	config := ocispec.Image{
		Config: ocispec.ImageConfig{
			Labels: annotations,
		},
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: []digest.Digest{digest.FromBytes(blobData)},
		},
		History: []ocispec.History{{
			CreatedBy: "kpm",
		}},
		Platform: ocispec.Platform{
			OS: "linux",
		},
	}
	return json.Marshal(config)
}

func getBlobData(manifestsFS, metadataFS fs.FS) ([]byte, error) {
	buf := &bytes.Buffer{}
	gzw := gzip.NewWriter(buf)

	if err := tar.Directory(gzw, &registryFS{
		manifestsFS: manifestsFS,
		metadataFS:  metadataFS,
	}); err != nil {
		return nil, err
	}
	if err := gzw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type registryFS struct {
	manifestsFS fs.FS
	metadataFS  fs.FS
}

func (r *registryFS) Open(name string) (fs.File, error) {
	name = filepath.Clean(name)
	if name == "." {
		return fstest.MapFS{"manifests": &fstest.MapFile{Mode: fs.ModeDir}, "metadata": &fstest.MapFile{Mode: fs.ModeDir}}.Open(name)
	}
	if name == "manifests" {
		return r.manifestsFS.Open(".")
	}
	if name == "metadata" {
		return r.metadataFS.Open(".")
	}

	if manifestsPrefix := "manifests" + string(filepath.Separator); strings.HasPrefix(name, manifestsPrefix) {
		return r.manifestsFS.Open(strings.TrimPrefix(name, manifestsPrefix))
	}
	if metadataPrefix := "metadata" + string(filepath.Separator); strings.HasPrefix(name, metadataPrefix) {
		return r.metadataFS.Open(strings.TrimPrefix(name, metadataPrefix))
	}

	return nil, fs.ErrNotExist
}
