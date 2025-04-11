package spec

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing/fstest"

	"github.com/blang/semver/v4"
	"github.com/containers/image/v5/docker/reference"
	"github.com/google/renameio"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/operator-framework/api/pkg/operators/v1alpha1"

	specsv1 "github.com/joelanford/kpm/internal/experimental/api/specs/v1"
	"github.com/joelanford/kpm/internal/kpm"
)

type registryV1 struct {
	root        fs.FS
	name        string
	version     semver.Version
	release     string
	annotations map[string]string
}

func (rv1 *registryV1) Build(ref reference.NamedTagged, outputDir string) error {
	outName := filepath.Join(outputDir, fmt.Sprintf("%s.v%s-%s.registryv1.kpm", rv1.name, rv1.version.String(), rv1.release))
	outFile, err := renameio.TempFile(outputDir, outName)
	if err != nil {
		return err
	}
	if _, err := kpm.WriteImageManifest(outFile, ref, []fs.FS{rv1.root}, rv1.annotations); err != nil {
		return errors.Join(err, outFile.Cleanup())
	}
	if err := outFile.CloseAtomicallyReplace(); err != nil {
		return errors.Join(err, outFile.Cleanup())
	}
	return nil
}

const (
	annotationsFile     = "metadata/annotations.yaml"
	annotationMediaType = "operators.operatorframework.io.bundle.mediatype.v1"
	annotationPackage   = "operators.operatorframework.io.bundle.package.v1"
	annotationManifests = "operators.operatorframework.io.bundle.manifests.v1"
)

func loadRegistryV1Bytes(specData []byte, workingDir string, imageOverrides map[string]string) (Spec, error) {
	var rv1Spec specsv1.RegistryV1
	if err := yaml.Unmarshal(specData, &rv1Spec, yaml.DisallowUnknownFields); err != nil {
		return nil, err
	}

	switch rv1Spec.Source.SourceType {
	case specsv1.RegistryV1SourceTypeBundleDirectory:
		bundleRoot := os.DirFS(filepath.Join(workingDir, rv1Spec.Source.BundleDirectory.Path))
		packageName, manifestsDir, annotations, err := parseRegistryV1Metadata(bundleRoot)
		if err != nil {
			return nil, fmt.Errorf("error reading bundle metadata: %v", err)
		}

		version, relatedImages, err := parseRegistryV1Manifests(bundleRoot, manifestsDir)
		if err != nil {
			return nil, fmt.Errorf("error reading bundle manifests: %v", err)
		}

		var imageReplacements []string
		for name, original := range relatedImages {
			replacement, ok := imageOverrides[name]
			if ok {
				imageReplacements = append(imageReplacements, original, replacement)
			}
		}
		replacer := strings.NewReplacer(imageReplacements...)

		for k, v := range annotations {
			delete(annotations, k)
			annotations[replacer.Replace(k)] = replacer.Replace(v)
		}

		root := fstest.MapFS{}
		if err := fs.WalkDir(bundleRoot, ".", func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				root[path] = &fstest.MapFile{Mode: fs.ModeDir | 0700}
				return nil
			}
			fileData, err := fs.ReadFile(bundleRoot, path)
			if err != nil {
				return err
			}
			replacedFileData := []byte(replacer.Replace(string(fileData)))
			root[path] = &fstest.MapFile{Data: replacedFileData, Mode: 0600}
			return nil
		}); err != nil {
			return nil, fmt.Errorf("failed to read bundle directory: %v", err)
		}

		rv1 := &registryV1{
			root:        root,
			name:        packageName,
			version:     *version,
			release:     rv1Spec.Release,
			annotations: annotations,
		}
		return rv1, nil
	case specsv1.RegistryV1SourceTypeManifests:
		panic("not implemented")
	case specsv1.RegistryV1SourceTypeKustomization:
		panic("not implemented")
	}
	return nil, fmt.Errorf("unknown source type %q", rv1Spec.Source.SourceType)
}

func parseRegistryV1Metadata(bundleFS fs.FS) (string, string, map[string]string, error) {
	// read annotations file
	annotationsData, err := fs.ReadFile(bundleFS, annotationsFile)
	if err != nil {
		return "", "", nil, fmt.Errorf("failed to read file %q: %w", annotationsFile, err)
	}

	// parse annotations file
	var annotations struct {
		Annotations map[string]string `json:"annotations"`
	}
	if err := yaml.Unmarshal(annotationsData, &annotations); err != nil {
		return "", "", nil, fmt.Errorf("failed to parse file %q: %w", annotationsFile, err)
	}
	if annotations.Annotations == nil {
		return "", "", nil, fmt.Errorf("annotations not found in %q", annotationsFile)
	}

	// verify mediatype
	var errs []error
	mediaType, ok := annotations.Annotations[annotationMediaType]
	if !ok {
		errs = append(errs, fmt.Errorf("media type key %q not found in %q", annotationMediaType, annotationsFile))
	} else if mediaType != "registry+v1" {
		errs = append(errs, fmt.Errorf("unsupported media type %q", mediaType))
	}

	// get package name
	packageName, ok := annotations.Annotations[annotationPackage]
	if !ok {
		errs = append(errs, fmt.Errorf("package name key %q not found in %q", annotationPackage, annotationsFile))
	}

	manifestsDir, ok := annotations.Annotations[annotationManifests]
	if !ok {
		errs = append(errs, fmt.Errorf("manifests key %q not found in %q", annotationManifests, annotationsFile))
	}
	if len(errs) > 0 {
		return "", "", nil, errors.Join(errs...)
	}

	return packageName, manifestsDir, annotations.Annotations, nil
}

func parseRegistryV1Manifests(bundleFS fs.FS, manifestsDir string) (*semver.Version, map[string]string, error) {
	manifestsEntries, err := fs.ReadDir(bundleFS, filepath.Clean(manifestsDir))
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read directory %q: %w", manifestsDir, err)
	}

	var (
		foundCSV      bool
		version       *semver.Version
		relatedImages []v1alpha1.RelatedImage
		errs          []error
	)
	for _, manifestEntry := range manifestsEntries {
		path := filepath.Join(manifestsDir, manifestEntry.Name())
		if manifestEntry.IsDir() {
			errs = append(errs, fmt.Errorf("unexpected directory %q", path))
			continue
		}

		manifestData, err := fs.ReadFile(bundleFS, filepath.Join(manifestsDir, manifestEntry.Name()))
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read file %q: %v", path, err))
			continue
		}

		// parse manifest
		var u unstructured.Unstructured
		if err := yaml.Unmarshal(manifestData, &u); err != nil {
			errs = append(errs, fmt.Errorf("failed to parse file %q: %v", path, err))
			continue
		}

		if u.GroupVersionKind().Kind == "ClusterServiceVersion" {
			if foundCSV {
				errs = append(errs, fmt.Errorf("multiple ClusterServiceVersion objects found in %q", manifestsDir))
				continue
			}
			foundCSV = true
			var csv v1alpha1.ClusterServiceVersion
			if err := yaml.Unmarshal(manifestData, &csv); err != nil {
				errs = append(errs, fmt.Errorf("failed to parse ClusterServiceVersion in file %q: %v", path, err))
				continue
			}
			version = &csv.Spec.Version.Version
			relatedImages = csv.Spec.RelatedImages
		}
	}
	if !foundCSV {
		errs = append(errs, fmt.Errorf("no ClusterServiceVersion objects found in %q", manifestsDir))
	}
	if len(errs) > 0 {
		return nil, nil, errors.Join(errs...)
	}

	return version, registryV1RelatedImagesMap(relatedImages), nil
}

func registryV1RelatedImagesMap(relatedImages []v1alpha1.RelatedImage) map[string]string {
	m := make(map[string]string)
	for _, relatedImage := range relatedImages {
		if relatedImage.Name == "" {
			continue
		}
		m[relatedImage.Name] = relatedImage.Image
	}
	return m
}

func init() {
	if err := registerKind(specsv1.GroupVersion.WithKind(specsv1.KindRegistryV1), loadRegistryV1Bytes); err != nil {
		panic(err)
	}
}
