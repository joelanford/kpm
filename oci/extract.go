package oci

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/google/go-containerregistry/pkg/v1/layout"
	"github.com/joelanford/kpm/internal/fsutil"
	"github.com/nlepage/go-tarfs"
)

func Extract(ctx context.Context, art Artifact) (fs.FS, error) {
	pr, pw := io.Pipe()
	go func() {
		defer pw.Close()
		if _, _, err := Write(ctx, pw, art); err != nil {
			pw.CloseWithError(err)
		}
	}()
	tr, err := tarfs.New(pr)
	if err != nil {
		return nil, err
	}
	tmpDir, err := os.MkdirTemp("", "oci-layout-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)

	if err := fsutil.Write(tmpDir, tr); err != nil {
		return nil, err
	}
	_ = pr.Close()

	path, err := layout.FromPath(tmpDir)
	if err != nil {
		return nil, err
	}
	idx, err := path.ImageIndex()
	if err != nil {
		return nil, err
	}
	idxManifest, err := idx.IndexManifest()
	if err != nil {
		return nil, err
	}
	if len(idxManifest.Manifests) != 1 {
		return nil, fmt.Errorf("found %d manifests in index, expected 1", len(idxManifest.Manifests))
	}
	bundleHash := idxManifest.Manifests[0].Digest

	img, err := idx.Image(bundleHash)
	if err != nil {
		return nil, err
	}

	pr, pw = io.Pipe()
	go func() {
		defer pw.Close()
		if err := crane.Export(img, pw); err != nil {
			pw.CloseWithError(err)
		}
	}()
	return tarfs.New(pr)
}
