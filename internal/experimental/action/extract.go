package action

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/joelanford/kpm/internal/experimental/kpm"
)

func Extract(ctx context.Context, kpmFilePath string, outputDir string) error {
	if outputDir == "" {
		if strings.HasSuffix(kpmFilePath, ".bundle.kpm") {
			outputDir = strings.TrimSuffix(kpmFilePath, ".bundle.kpm")
		} else if strings.HasSuffix(kpmFilePath, ".kpm") {
			outputDir = strings.TrimSuffix(kpmFilePath, ".kpm")
		} else {
			return fmt.Errorf("unexpected kpm file extension %q: expected %q or %q", filepath.Ext(kpmFilePath), ".bundle.kpm", ".kpm")
		}
	}
	kpmFile, err := kpm.Open(ctx, kpmFilePath)
	if err != nil {
		return fmt.Errorf("failed to open kpm file %q: %w", kpmFilePath, err)
	}
	if _, err := kpmFile.Mount(outputDir); err != nil {
		return fmt.Errorf("failed to mount kpm file %q: %w", kpmFilePath, err)
	}

	return nil
}
