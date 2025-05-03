package spec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/renameio/v2"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content/oci"

	bundlev1alpha1 "github.com/joelanford/kpm/internal/api/bundle/v1alpha1"
	"github.com/joelanford/kpm/internal/pkg/util/tar"
)

type BuildReport struct {
	ID         bundlev1alpha1.ID  `json:"id"`
	Descriptor ocispec.Descriptor `json:"descriptor"`
	OutputFile string             `json:"outputFile"`
}

func (r BuildReport) WriteFile(reportFile string) error {
	reportData, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal report: %v", err)
	}
	if reportFile == "-" || reportFile == "/dev/stdout" {
		fmt.Println(string(reportData))
	}
	return renameio.WriteFile(reportFile, reportData, 0644)
}

func Build(ctx context.Context, spec Spec) (*BuildReport, error) {
	tmpDir, err := os.MkdirTemp("", "kpm-build-bundle-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	pusher, err := oci.NewWithContext(ctx, tmpDir)
	if err != nil {
		return nil, err
	}

	bundleDesc, err := spec.MarshalOCI(ctx, pusher)
	if err != nil {
		return nil, err
	}

	id := spec.ID()
	idData, err := json.Marshal(id)
	if err != nil {
		return nil, err
	}
	idDesc, err := oras.PushBytes(ctx, pusher, bundlev1alpha1.MediaTypeID, idData)
	if err != nil {
		return nil, err
	}
	idManifest := ocispec.Manifest{
		Versioned: specs.Versioned{SchemaVersion: 2},
		MediaType: ocispec.MediaTypeImageManifest,
		Config:    ocispec.DescriptorEmptyJSON,
		Layers:    []ocispec.Descriptor{idDesc},
		Subject:   &bundleDesc,
	}
	idManifestJSON, err := json.Marshal(idManifest)
	if err != nil {
		return nil, err
	}
	if _, err := oras.PushBytes(ctx, pusher, ocispec.MediaTypeEmptyJSON, ocispec.DescriptorEmptyJSON.Data); err != nil {
		return nil, err
	}
	idManifestDesc, err := oras.PushBytes(ctx, pusher, ocispec.MediaTypeImageManifest, idManifestJSON)
	if err != nil {
		return nil, err
	}

	if err := pusher.Tag(ctx, idManifestDesc, fmt.Sprintf("%s.id", id.String())); err != nil {
		return nil, err
	}
	if err := pusher.Tag(ctx, bundleDesc, id.String()); err != nil {
		return nil, err
	}

	outputFile := id.Filename()
	pf, err := renameio.NewPendingFile(outputFile)
	if err != nil {
		return nil, err
	}
	defer pf.Cleanup()

	if err := tar.Directory(pf, os.DirFS(tmpDir)); err != nil {
		return nil, err
	}
	if err := pf.CloseAtomicallyReplace(); err != nil {
		return nil, err
	}

	return &BuildReport{
		ID:         id,
		Descriptor: bundleDesc,
		OutputFile: outputFile,
	}, nil
}
