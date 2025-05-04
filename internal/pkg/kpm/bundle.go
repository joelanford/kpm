package kpm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/containers/image/v5/docker/reference"
	"github.com/nlepage/go-tarfs"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	bundlev1alpha1 "github.com/joelanford/kpm/internal/api/bundle/v1alpha1"
	"github.com/joelanford/kpm/internal/pkg/remote"
)

type Bundle struct {
	store *oci.ReadOnlyStore
	id    bundlev1alpha1.ID

	idDesc     ocispec.Descriptor
	bundleDesc ocispec.Descriptor
}

func OpenBundle(ctx context.Context, path string) (*Bundle, error) {
	store, err := oci.NewFromTar(ctx, path)
	if err != nil {
		return nil, err
	}

	idx, err := readOCILayoutIndex(path)
	if err != nil {
		return nil, err
	}

	if len(idx.Manifests) != 2 {
		return nil, fmt.Errorf("expected exactly two manifests in KPM bundle index, got %d", len(idx.Manifests))
	}

	// Read the ID manifest descriptor
	idManifestDesc := idx.Manifests[0]
	idManifestReader, err := store.Fetch(ctx, idManifestDesc)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ID manifest: %v", err)
	}
	defer idManifestReader.Close()
	idManifestData, err := io.ReadAll(idManifestReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read ID manifest: %v", err)
	}
	var idManifest ocispec.Manifest
	if err := json.Unmarshal(idManifestData, &idManifest); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ID manifest: %v", err)
	}

	// Read the ID
	var idDesc *ocispec.Descriptor
	for _, desc := range idManifest.Layers {
		if desc.MediaType == bundlev1alpha1.MediaTypeID {
			idDesc = &desc
			break
		}
	}
	if idDesc == nil {
		return nil, fmt.Errorf("failed to find ID manifest in KPM bundle index")
	}
	idReader, err := store.Fetch(ctx, *idDesc)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch ID data: %v", err)
	}
	defer idReader.Close()
	idData, err := io.ReadAll(idReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read ID data: %v", err)
	}
	var id bundlev1alpha1.ID
	if err := json.Unmarshal(idData, &id); err != nil {
		return nil, fmt.Errorf("failed to unmarshal ID: %v", err)
	}

	return &Bundle{
		store:      store,
		idDesc:     idManifestDesc,
		bundleDesc: idx.Manifests[1],
		id:         id,
	}, nil
}

func (k *Bundle) Descriptor() ocispec.Descriptor {
	return k.bundleDesc
}

func (k *Bundle) Push(ctx context.Context, namespace string, opts oras.CopyGraphOptions) (reference.NamedTagged, error) {
	ref, err := k.id.FullyQualifiedReference(namespace)
	if err != nil {
		return nil, fmt.Errorf("failed to build reference: %v", err)
	}

	target, err := remote.NewRepository(ref.Name())
	if err != nil {
		return ref, fmt.Errorf("failed to make remote registry client: %v", err)
	}

	if err := oras.CopyGraph(ctx, k.store, target, k.bundleDesc, opts); err != nil {
		return ref, fmt.Errorf("failed to copy kpm to destination: %w", err)
	}
	if err := target.Tag(ctx, k.bundleDesc, ref.Tag()); err != nil {
		return ref, fmt.Errorf("failed to tag copied kpm bundle: %w", err)
	}

	if err := oras.CopyGraph(ctx, k.store, target, k.idDesc, opts); err != nil {
		return ref, fmt.Errorf("failed to copy kpm to destination: %w", err)
	}
	if err := target.Tag(ctx, k.idDesc, fmt.Sprintf("%s.id", ref.Tag())); err != nil {
		return ref, fmt.Errorf("failed to tag copied kpm bundle ID: %w", err)
	}
	return ref, nil
}

func readOCILayoutIndex(layoutRootPath string) (*ocispec.Index, error) {
	kpmFile, err := os.Open(layoutRootPath)
	if err != nil {
		return nil, err
	}
	defer kpmFile.Close()

	kpmFS, err := tarfs.New(kpmFile)
	if err != nil {
		return nil, err
	}

	layoutIndexData, err := fs.ReadFile(kpmFS, "index.json")
	if err != nil {
		return nil, fmt.Errorf("failed to open layout index: %w", err)
	}
	var layoutIndex ocispec.Index
	if err := json.Unmarshal(layoutIndexData, &layoutIndex); err != nil {
		return nil, fmt.Errorf("failed to unmarshal layout index: %w", err)
	}
	return &layoutIndex, nil
}
