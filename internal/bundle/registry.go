package bundle

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"testing/fstest"

	"github.com/blang/semver/v4"
	"github.com/containers/storage/pkg/archive"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"golang.org/x/exp/maps"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"oras.land/oras-go/v2/content"
	"sigs.k8s.io/yaml"

	"github.com/joelanford/kpm/internal/kpm"
)

type registryV1 struct {
	root fs.FS

	packageName string
	version     semver.Version
	annotations map[string]string
}

func NewRegistry(root fs.FS, extraAnnotations map[string]string) (Bundle, error) {
	b := &registryV1{root: root}
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

func (b *registryV1) PackageName() string {
	return b.packageName
}

func (b *registryV1) Version() semver.Version {
	return b.version
}

func (b *registryV1) Annotations() map[string]string {
	return b.annotations
}

func (b *registryV1) MarshalOCI(ctx context.Context, pusher content.Pusher) (ocispec.Descriptor, error) {
	return kpm.MarshalOCIManifest(ctx, pusher, []fs.FS{b.root}, b.annotations)
}

func (b *registryV1) UnmarshalOCI(ctx context.Context, fetcher content.Fetcher, desc ocispec.Descriptor) error {
	manifestData, err := content.FetchAll(ctx, fetcher, desc)
	if err != nil {
		return err
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "kpm-unmarshal-fs")
	for _, layer := range manifest.Layers {
		if err := func() error {
			lr, err := fetcher.Fetch(ctx, layer)
			if err != nil {
				return err
			}
			defer lr.Close()
			vr := content.NewVerifyReader(lr, layer)
			_, err = archive.ApplyLayer(tmpDir, vr)
			return err
		}(); err != nil {
			return errors.Join(err, os.RemoveAll(tmpDir))
		}
	}

	vfs := fstest.MapFS{}
	if err := filepath.WalkDir(tmpDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		fi, err := d.Info()
		if err != nil {
			return err
		}
		vfs[path] = &fstest.MapFile{
			Mode:    fi.Mode(),
			ModTime: fi.ModTime(),
			Sys:     fi.Sys(),
		}
		if d.IsDir() {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		vfs[path].Data = data
		return nil
	}); err != nil {
		return err
	}
	b.root = vfs
	if err := b.parseMetadata(); err != nil {
		return err
	}
	if err := b.parseManifests(); err != nil {
		return err
	}
	b.annotations = manifest.Annotations
	return nil
}

const (
	manifestsDirectory = "manifests"
	annotationsFile    = "metadata/annotations.yaml"
	mediaTypeKey       = "operators.operatorframework.io.bundle.mediatype.v1"
	packageNameKey     = "operators.operatorframework.io.bundle.package.v1"
)

func (b *registryV1) parseMetadata() error {
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

func (b *registryV1) parseManifests() error {
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
