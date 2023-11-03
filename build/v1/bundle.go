package v1

import (
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"sigs.k8s.io/yaml"

	v1 "github.com/joelanford/kpm/api/v1"
)

func Bundle(bundleSpecReader io.Reader, workingFs fs.FS) (*v1.Bundle, error) {
	bundleSpecData, err := io.ReadAll(bundleSpecReader)
	if err != nil {
		return nil, fmt.Errorf("read bundle spec: %w", err)
	}
	var bundleSpec v1.BundleSpec
	if err := yaml.Unmarshal(bundleSpecData, &bundleSpec); err != nil {
		return nil, fmt.Errorf("unmarshal bundle spec: %w", err)
	}
	bundleSpec.BundleConfig.Provides = append(bundleSpec.BundleConfig.Provides, fmt.Sprintf("package(%s=%s)", bundleSpec.BundleConfig.Name, bundleSpec.BundleConfig.Version))

	bundle := &v1.Bundle{
		BundleConfig:     bundleSpec.BundleConfig,
		ExtraAnnotations: bundleSpec.Annotations,
	}

	if bundleSpec.Source != nil {
		var (
			contentData []byte
			err         error
		)
		switch bundleSpec.Source.Type {
		case "file":
			fileName := filepath.Clean(bundleSpec.Source.File.Path)
			contentData, err = fs.ReadFile(workingFs, fileName)
		default:
			return nil, fmt.Errorf("unsupported source type: %s", bundleSpec.Source.Type)
		}
		if err != nil {
			return nil, fmt.Errorf("read bundle content: %w", err)
		}
		bundle.ContentMediaType = bundleSpec.Source.MediaType
		bundle.Content = contentData
	}

	if err := bundle.Validate(); err != nil {
		return nil, fmt.Errorf("invalid bundle: %w", err)
	}

	return bundle, nil
}
