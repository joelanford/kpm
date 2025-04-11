package spec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/google/renameio/v2"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/oci"

	"github.com/joelanford/kpm/internal/pkg/util/tar"
)

type BuildReport struct {
	ID         string             `json:"id"`
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

	ociLayout, err := oci.NewWithContext(ctx, tmpDir)
	if err != nil {
		return nil, err
	}

	desc, err := spec.MarshalOCI(ctx, ociLayout)
	if err != nil {
		return nil, err
	}

	id := spec.ID()
	outputFile := fmt.Sprintf("%s.kpm", id)
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
		Descriptor: desc,
		OutputFile: outputFile,
	}, nil
}
