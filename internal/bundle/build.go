package bundle

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/containers/image/v5/docker/reference"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/joelanford/kpm/internal/kpm"
)

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

	// Write it!
	desc, err := kpm.WriteImageManifest(w, tagRef, []fs.FS{b.FS()}, b.Annotations())
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to write kpm file: %v", err)
	}
	return tagRef, desc, nil
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
