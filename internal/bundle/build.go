package bundle

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"

	"github.com/blang/semver/v4"
	"github.com/containers/image/v5/docker/reference"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"sigs.k8s.io/yaml"

	specsv1 "github.com/joelanford/kpm/internal/api/specs/v1"
	"github.com/joelanford/kpm/internal/kpm"
)

const (
	MediaTypeOLMOperatorFrameworkRegistryV1 = "olm.operatorframework.io/registry+v1"
)

type BuildResult struct {
	FilePath    string             `json:"filePath"`
	Repository  string             `json:"imageRepository"`
	Tag         string             `json:"imageTag"`
	Descriptor  ocispec.Descriptor `json:"imageDescriptor"`
	PackageName string             `json:"bundlePackageName"`
	Version     semver.Version     `json:"bundleVersion"`
}

func BuildFromSpecFile(ctx context.Context, specFileName string, filenameFunc func(Bundle) (string, error)) (*BuildResult, error) {
	spec, err := ReadSpecFile(specFileName)
	if err != nil {
		return nil, fmt.Errorf("failed to read spec: %w", err)
	}

	b, err := LoadFromSpec(*spec, filepath.Dir(specFileName))
	if err != nil {
		return nil, fmt.Errorf("failed to load bundle: %v", err)
	}

	outputFile, err := filenameFunc(b)
	if err != nil {
		return nil, fmt.Errorf("failed to generate output file name: %v", err)
	}
	imageRef, err := StringFromBundleTemplate(spec.ImageReference)(b)
	if err != nil {
		return nil, fmt.Errorf("failed to generate image reference: %v", err)
	}
	tagRef, desc, err := kpm.BuildFile(ctx, outputFile, b, imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to build kpm bundle: %v", err)
	}

	res := &BuildResult{
		FilePath:    outputFile,
		Repository:  reference.TrimNamed(tagRef).String(),
		Tag:         tagRef.Tag(),
		Descriptor:  desc,
		PackageName: b.PackageName(),
		Version:     b.Version(),
	}

	return res, nil
}

func LoadFromSpec(spec specsv1.Bundle, baseDir string) (Bundle, error) {
	// Load the bundle
	path := filepath.Join(baseDir, spec.Path)
	if filepath.IsAbs(spec.Path) {
		path = spec.Path
	}

	switch spec.MediaType {
	case MediaTypeOLMOperatorFrameworkRegistryV1:
		return NewRegistry(os.DirFS(path), spec.ExtraAnnotations)
	case "application/vnd.cncf.helm.chart.v1.tar+gzip":
		return NewHelm(path, spec.ExtraAnnotations)
	default:
		return nil, fmt.Errorf("unsupported media type: %s", spec.MediaType)
	}
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

func StringFromBundleTemplate(tmplStr string) func(b Bundle) (string, error) {
	return func(b Bundle) (string, error) {
		tmpl, err := template.New("").Delims("{", "}").Parse(tmplStr)
		if err != nil {
			return "", fmt.Errorf("invalid template %q: %w", tmplStr, err)
		}
		tmplData := map[string]string{
			"PackageName": b.PackageName(),
			"Version":     b.Version().String(),
		}
		var buf bytes.Buffer
		if err := tmpl.Execute(&buf, tmplData); err != nil {
			return "", fmt.Errorf("failed to render template %q: %w", tmplStr, err)
		}

		return buf.String(), nil
	}
}
