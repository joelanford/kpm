package v1

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"io/fs"

	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/tar"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/yaml"
)

func LegacyRegistryBundle(rootFS fs.FS) (*v1.LegacyRegistryBundle, error) {
	name, err := getLegacyRegistryBundleName(rootFS)
	if err != nil {
		return nil, err
	}

	annotations, err := readAnnotations(rootFS)
	if err != nil {
		return nil, err
	}

	blobData, err := getBlobData(rootFS)
	if err != nil {
		return nil, err
	}

	configData, err := getConfigData(annotations, blobData)
	if err != nil {
		return nil, err
	}

	return v1.NewLegacyRegistryBundle(name, configData, blobData, annotations), nil
}

func getLegacyRegistryBundleName(workingFs fs.FS) (string, error) {
	manifestsFS, err := fs.Sub(workingFs, "manifests")
	if err != nil {
		return "", err
	}

	manifestFiles, err := fs.ReadDir(manifestsFS, ".")
	if err != nil {
		return "", err
	}

	errs := []error{}
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
			return m.Name, nil
		}
	}
	return "", errors.Join(errs...)
}

func readAnnotations(workingFs fs.FS) (map[string]string, error) {
	annotationsFile, err := fs.ReadFile(workingFs, "metadata/annotations.yaml")
	if err != nil {
		return nil, err
	}
	var annotations struct {
		Annotations map[string]string `yaml:"annotations"`
	}
	if err := yaml.Unmarshal(annotationsFile, &annotations); err != nil {
		return nil, err
	}
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

func getBlobData(workingFs fs.FS) ([]byte, error) {
	buf := &bytes.Buffer{}
	gzw := gzip.NewWriter(buf)
	if err := tar.Directory(gzw, workingFs); err != nil {
		return nil, err
	}
	if err := gzw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
