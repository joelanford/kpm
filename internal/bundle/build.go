package bundle

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"

	"github.com/containers/image/v5/docker/reference"
	specsv1 "github.com/joelanford/kpm/internal/api/specs/v1"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"sigs.k8s.io/yaml"
)

const (
	MediaTypeOLMOperatorFrameworkRegistryV1 = "olm.operatorframework.io/registry+v1"
)

func BuildFromSpecFile(specFileName string, filenameFunc func(Bundle) (string, error), bundleHooks ...func(Bundle) error) (string, reference.NamedTagged, ocispec.Descriptor, error) {
	spec, err := ReadSpecFile(specFileName)
	if err != nil {
		return "", nil, ocispec.Descriptor{}, fmt.Errorf("failed to read spec: %w", err)
	}

	b, err := LoadFromSpec(*spec, filepath.Dir(specFileName))
	if err != nil {
		return "", nil, ocispec.Descriptor{}, fmt.Errorf("failed to load registry bundle: %v", err)
	}

	for _, hook := range bundleHooks {
		if err := hook(b); err != nil {
			return "", nil, ocispec.Descriptor{}, fmt.Errorf("failed to apply hooks: %v", err)
		}
	}

	outputFile, err := filenameFunc(b)
	tagRef, desc, err := BuildFile(outputFile, b, spec.RegistryNamespace)
	if err != nil {
		return "", nil, ocispec.Descriptor{}, fmt.Errorf("failed to build kpm bundle: %v", err)
	}
	return outputFile, tagRef, desc, nil
}

func LoadFromSpec(spec specsv1.Bundle, baseDir string) (Bundle, error) {
	// Load the bundle
	bundleDir := filepath.Join(baseDir, spec.BundleRoot)
	if filepath.IsAbs(spec.BundleRoot) {
		bundleDir = spec.BundleRoot
	}

	switch spec.MediaType {
	case MediaTypeOLMOperatorFrameworkRegistryV1:
		return NewRegistry(os.DirFS(bundleDir))
	default:
		return nil, fmt.Errorf("unsupported media type: %s", spec.MediaType)
	}
}

func BuildFile(fileName string, b Bundle, registryNamespace string) (reference.NamedTagged, ocispec.Descriptor, error) {
	file, err := os.Create(fileName)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to create output file: %v", err)
	}
	tagRef, desc, err := BuildWriter(file, b, registryNamespace)
	if err != nil {
		return nil, ocispec.Descriptor{}, errors.Join(err, os.Remove(fileName))
	}
	return tagRef, desc, nil
}

// BuildWriter writes a bundle to a writer
func BuildWriter(w io.Writer, b Bundle, registryNamespace string) (reference.NamedTagged, ocispec.Descriptor, error) {
	tagRef, err := getBundleRef(b, registryNamespace)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to get tagged reference from spec file: %w", err)
	}

	desc, err := b.WriteOCIArchive(w, tagRef)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to write kpm file: %v", err)
	}
	return tagRef, desc, nil
}

func ReadSpecFile(specFileName string) (*specsv1.Bundle, error) {
	f, err := os.Open(specFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to open spec file: %v", err)
	}
	defer f.Close()
	return ReadSpec(f)
}

func ReadSpec(specReader io.Reader) (*specsv1.Bundle, error) {
	specData, err := io.ReadAll(specReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec: %w", err)
	}

	var spec specsv1.Bundle
	if err := yaml.Unmarshal(specData, &spec); err != nil {
		return nil, fmt.Errorf("failed to parse bundle spec: %w", err)
	}
	expectedGVK := specsv1.GroupVersion.WithKind(specsv1.KindBundle)
	if spec.GroupVersionKind() != expectedGVK {
		return nil, fmt.Errorf("unsupported spec API found: %s, expected %s", spec.GroupVersionKind(), expectedGVK)
	}
	return &spec, nil
}

func getBundleRef(b Bundle, registryNamespace string) (reference.NamedTagged, error) {
	repoShortName := fmt.Sprintf("%s-bundle", b.PackageName())
	repoName := fmt.Sprintf("%s/%s", registryNamespace, repoShortName)
	nameRef, err := reference.ParseNamed(repoName)
	if err != nil {
		return nil, fmt.Errorf("failed to parse repository name %q: %v", err)
	}
	tag := fmt.Sprintf("v%s", b.Version())
	return reference.WithTag(nameRef, tag)
}

func FilenameFromTemplate(tmplStr string) func(b Bundle) (string, error) {
	return func(b Bundle) (string, error) {
		fileNameTmpl, err := template.New("filename").Delims("{", "}").Parse(tmplStr)
		if err != nil {
			return "", fmt.Errorf("invalid filename template %q: %w", tmplStr, err)
		}
		tmplData := map[string]string{
			"PackageName": b.PackageName(),
			"Version":     b.Version().String(),
		}
		var fileNameBuf bytes.Buffer
		if err := fileNameTmpl.Execute(&fileNameBuf, tmplData); err != nil {
			return "", fmt.Errorf("failed to render filename template %q: %w", fileNameTmpl, err)
		}

		return fileNameBuf.String(), nil
	}
}
