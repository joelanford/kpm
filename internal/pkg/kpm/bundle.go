package kpm

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"

	"github.com/containers/image/v5/docker/reference"
	"github.com/nlepage/go-tarfs"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	"github.com/joelanford/kpm/internal/pkg/remote"
)

type Bundle struct {
	store *oci.ReadOnlyStore

	tag  string
	desc ocispec.Descriptor
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

	if len(idx.Manifests) != 1 {
		return nil, fmt.Errorf("expected exactly one descriptor in KPM bundle index, got %d", len(idx.Manifests))
	}

	desc := idx.Manifests[0]
	tag, ok := desc.Annotations[ocispec.AnnotationRefName]
	if !ok {
		return nil, fmt.Errorf("no tag found on bundle descriptor")
	}

	return &Bundle{
		store: store,
		tag:   tag,
		desc:  desc,
	}, nil
}

func (k *Bundle) Descriptor() ocispec.Descriptor {
	return k.desc
}

func (k *Bundle) Push(ctx context.Context, namespace string, opts oras.CopyGraphOptions) (reference.NamedTagged, error) {
	ref, err := buildTagRaf(namespace, k.tag)
	if err != nil {
		return nil, fmt.Errorf("failed to build reference: %v", err)
	}

	target, err := remote.NewRepository(ref.Name())
	if err != nil {
		return ref, fmt.Errorf("failed to make remote registry client: %v", err)
	}

	if err := oras.CopyGraph(ctx, k.store, target, k.desc, opts); err != nil {
		return ref, fmt.Errorf("failed to copy kpm to destination: %w", err)
	}
	if err := target.Tag(ctx, k.desc, ref.Tag()); err != nil {
		return ref, fmt.Errorf("failed to tag copied kpm bundle: %w", err)
	}

	if err := oras.CopyGraph(ctx, k.store, target, k.desc, opts); err != nil {
		return ref, fmt.Errorf("failed to copy kpm to destination: %w", err)
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

func buildTagRaf(namespace, refName string) (reference.NamedTagged, error) {
	refStr := path.Join(namespace, refName)
	ref, err := reference.Parse(refStr)
	if err != nil {
		return nil, fmt.Errorf("cannot construct valid reference from namespace %q and kpm reference name %q: %v", namespace, refName, err)
	}
	tagRef, ok := ref.(reference.NamedTagged)
	if !ok {
		return nil, fmt.Errorf("reference constructed from namespace %q and kpm reference name %q was not tagged", namespace, refName)
	}
	return tagRef, nil
}
