package v1

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/tar"
	"github.com/joelanford/kpm/oci"
	"sigs.k8s.io/yaml"
)

func Bundle(bundleSpecReader io.Reader, workingFs fs.FS) (oci.Artifact, error) {
	// Read the bundle spec into a byte slice for unmarshalling.
	bundleSpecData, err := io.ReadAll(bundleSpecReader)
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
		return buildRegistryV1(*bundleSpec.RegistryV1, workingFs)
	case "bundle":
		return buildBundle(*bundleSpec.Bundle, workingFs)
	}
	return nil, fmt.Errorf("unsupported bundle source type: %s", bundleSpec.Type)
}

func buildRegistryV1(spec v1.RegistryV1Source, workingFs fs.FS) (*v1.DockerImage, error) {
	return RegistryBundle(spec, workingFs)
}

func buildBundle(spec v1.BundleSource, workingFs fs.FS) (*v1.Bundle, error) {
	// Apply the implicit bundle provides
	ensureImplicitProvides(&spec.BundleConfig)

	bundle := &v1.Bundle{
		BundleConfig:     spec.BundleConfig,
		ExtraAnnotations: spec.Annotations,
		BundleContent: v1.BundleContent{
			ContentMediaType: spec.Source.MediaType,
		},
	}

	var err error
	switch spec.Source.Type {
	case "file":
		bundle.Content, err = getFileContent(workingFs, *spec.Source.File)
	case "dir":
		bundle.Content, err = getDirContent(workingFs, *spec.Source.Dir)
	default:
		return nil, fmt.Errorf("unsupported generic source type: %s", spec.Source.Type)
	}
	if err != nil {
		return nil, fmt.Errorf("read bundle content: %w", err)
	}
	if err := bundle.Validate(); err != nil {
		return nil, fmt.Errorf("invalid bundle: %w", err)
	}
	return bundle, nil
}

func ensureImplicitProvides(cfg *v1.BundleConfig) {
	implicitProvides := fmt.Sprintf("package(%s=%s)", cfg.Name, cfg.Version)
	found := false
	for _, p := range cfg.Provides {
		if p == implicitProvides {
			found = true
			break
		}
	}
	if !found {
		cfg.Provides = append(cfg.Provides, implicitProvides)
	}
}

func getFileContent(root fs.FS, file v1.BundleSourceFile) ([]byte, error) {
	fileName := filepath.Clean(file.Path)
	if filepath.IsAbs(fileName) {
		return nil, fmt.Errorf("absolute file paths are not allowed: %s", fileName)
	}
	return fs.ReadFile(root, fileName)
}

func getDirContent(root fs.FS, dir v1.BundleSourceDir) ([]byte, error) {
	dirName := filepath.Clean(dir.Path)
	if filepath.IsAbs(dirName) {
		return nil, fmt.Errorf("absolute directory paths are not allowed: %s", dirName)
	}
	dirFS, err := fs.Sub(root, dirName)
	if err != nil {
		return nil, fmt.Errorf("sub filesystem: %w", err)
	}
	buf := &bytes.Buffer{}
	if err := func() error {
		var w io.Writer = buf
		switch dir.Compression {
		case "gzip":
			gzw := gzip.NewWriter(w)
			w = gzw
			defer gzw.Close()
		case "":
			// no compression
		default:
			return fmt.Errorf("unsupported compression type: %s", dir.Compression)
		}
		switch dir.Archive {
		case "tar":
			if err := tar.Directory(w, dirFS); err != nil {
				return fmt.Errorf("tar directory: %w", err)
			}
		default:
			return fmt.Errorf("unsupported archive type: %s", dir.Archive)
		}
		return nil
	}(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
