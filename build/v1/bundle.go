package v1

import (
	"bytes"
	"compress/gzip"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"

	"sigs.k8s.io/yaml"

	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/tar"
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
			contentData, err = getFileContent(workingFs, *bundleSpec.Source.File)
		case "dir":
			contentData, err = getDirContent(workingFs, *bundleSpec.Source.Dir)
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
