package bundle

import (
	"errors"
	"fmt"
	"io"
	"io/fs"

	"github.com/blang/semver/v4"
	"github.com/containers/image/v5/docker/reference"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/kpm/internal/kpm"
)

type registry struct {
	root fs.FS

	packageName string
	version     semver.Version
	annotations map[string]string
}

func NewRegistry(root fs.FS, extraAnnotations map[string]string) (Bundle, error) {
	b := &registry{root: root}
	if extraAnnotations != nil {
		b.annotations = make(map[string]string, len(extraAnnotations))
		maps.Copy(b.annotations, extraAnnotations)
	}
	if err := b.parseMetadata(); err != nil {
		return nil, err
	}
	if err := b.parseManifests(); err != nil {
		return nil, err
	}
	return b, nil
}

func (b *registry) FS() fs.FS {
	return b.root
}

func (b *registry) PackageName() string {
	return b.packageName
}

func (b *registry) Version() semver.Version {
	return b.version
}

func (b *registry) Annotations() map[string]string {
	return b.annotations
}

func (b *registry) WriteOCIArchive(w io.Writer, name reference.NamedTagged) (ocispec.Descriptor, error) {
	return kpm.WriteImageManifest(w, name, kpm.ID{Name: b.packageName, Version: b.version, Release: "0"}, []fs.FS{b.root}, b.annotations, nil)
}

const (
	manifestsDirectory = "manifests"
	annotationsFile    = "metadata/annotations.yaml"
	mediaTypeKey       = "operators.operatorframework.io.bundle.mediatype.v1"
	packageNameKey     = "operators.operatorframework.io.bundle.package.v1"
)

func (b *registry) parseMetadata() error {
	// read annotations file
	annotationsData, err := fs.ReadFile(b.root, annotationsFile)
	if err != nil {
		return fmt.Errorf("failed to read file %q: %w", annotationsFile, err)
	}

	// parse annotations file
	var annotations struct {
		Annotations map[string]string `json:"annotations"`
	}
	if err := yaml.Unmarshal(annotationsData, &annotations); err != nil {
		return fmt.Errorf("failed to parse file %q: %w", annotationsFile, err)
	}
	if annotations.Annotations == nil {
		return fmt.Errorf("annotations not found in %q", annotationsFile)
	}

	// verify mediatype
	var errs []error
	mediaType, ok := annotations.Annotations[mediaTypeKey]
	if !ok {
		errs = append(errs, fmt.Errorf("media type key %q not found in %q", mediaTypeKey, annotationsFile))
	} else if mediaType != "registry+v1" {
		errs = append(errs, fmt.Errorf("unsupported media type %q", mediaType))
	}

	// get package name
	packageName, ok := annotations.Annotations[packageNameKey]
	if !ok {
		errs = append(errs, fmt.Errorf("package name key %q not found in %q", packageNameKey, annotationsFile))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	b.packageName = packageName
	if b.annotations == nil {
		b.annotations = make(map[string]string, len(annotations.Annotations))
	}
	maps.Copy(b.annotations, annotations.Annotations)
	return nil
}

func (b *registry) parseManifests() error {
	manifestsEntries, err := fs.ReadDir(b.root, manifestsDirectory)
	if err != nil {
		return fmt.Errorf("failed to read directory %q: %w", b.root, err)
	}

	var (
		foundCSV bool
		version  semver.Version
		errs     []error
	)
	for _, manifestEntry := range manifestsEntries {
		if manifestEntry.IsDir() {
			errs = append(errs, fmt.Errorf("unexpected directory %q in %q", manifestEntry.Name(), manifestsDirectory))
			continue
		}

		manifestData, err := fs.ReadFile(b.root, fmt.Sprintf("%s/%s", manifestsDirectory, manifestEntry.Name()))
		if err != nil {
			errs = append(errs, fmt.Errorf("failed to read file %q: %w", manifestEntry.Name(), err))
			continue
		}

		// parse manifest
		var u unstructured.Unstructured
		if err := yaml.Unmarshal(manifestData, &u); err != nil {
			errs = append(errs, fmt.Errorf("failed to parse file %q: %w", manifestEntry.Name(), err))
			continue
		}

		if u.GroupVersionKind().Kind == "ClusterServiceVersion" {
			foundCSV = true
			versionStr, ok, err := unstructured.NestedString(u.Object, "spec", "version")
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to get CSV version: %w", err))
				continue
			}
			if !ok {
				errs = append(errs, fmt.Errorf("CSV version not found"))
				continue
			}
			version, err = semver.Parse(versionStr)
			if err != nil {
				errs = append(errs, fmt.Errorf("failed to parse CSV version %q: %w", versionStr, err))
				continue
			}
		}
	}
	if !foundCSV {
		errs = append(errs, fmt.Errorf("no ClusterServiceVersion found in %q", manifestsDirectory))
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	b.version = version
	return nil
}
