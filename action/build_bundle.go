package action

import (
	"context"
	"io"
	"io/fs"
	"os"

	"oras.land/oras-go/v2/content/oci"

	buildv1 "github.com/joelanford/kpm/build/v1"
	"github.com/joelanford/kpm/internal/tar"
	kpmoci "github.com/joelanford/kpm/oci"
)

type BuildBundle struct {
	SpecFileReader    io.Reader
	SpecFileWorkingFS fs.FS

	BundleWriter io.Writer
}

func (a *BuildBundle) Run(ctx context.Context) error {
	bundle, err := buildv1.Bundle(a.SpecFileReader, a.SpecFileWorkingFS)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", "kpm-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	tmpStore, err := oci.NewWithContext(ctx, tmpDir)
	if err != nil {
		return err
	}

	desc, err := kpmoci.Push(ctx, bundle, tmpStore, kpmoci.PushOptions{})
	if err != nil {
		return err
	}

	tag := bundle.String()
	if err := tmpStore.Tag(ctx, desc, tag); err != nil {
		return err
	}

	return tar.Directory(a.BundleWriter, os.DirFS(tmpDir))
}
