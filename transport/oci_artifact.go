package transport

import (
	"context"
	"fmt"
	"github.com/joelanford/kpm/internal/tar"
	kpmoci "github.com/joelanford/kpm/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/oci"
	"os"
	"strings"
)

type OCIArchiveTransport struct{}

func init() {
	all.Register(&OCIArchiveTransport{})
}

func (t *OCIArchiveTransport) ParseReference(ref string) (Target, error) {
	filename, tag, _ := strings.Cut(ref, ":")
	return &OCIArchiveTarget{
		Filename: filename,
		Tag:      tag,
	}, nil
}

func (t *OCIArchiveTransport) Protocol() string {
	return "oci-archive"
}

type OCIArchiveTarget struct {
	Filename string
	Tag      string
}

func (t *OCIArchiveTarget) Push(ctx context.Context, artifact kpmoci.Artifact) (string, ocispec.Descriptor, error) {
	f, err := os.Create(t.Filename)
	if err != nil {
		return "", ocispec.Descriptor{}, err
	}

	tmpDir, err := os.MkdirTemp("", "kpm-")
	if err != nil {
		return "", ocispec.Descriptor{}, err
	}
	defer os.RemoveAll(tmpDir)

	tmpStore, err := oci.NewWithContext(ctx, tmpDir)
	if err != nil {
		return "", ocispec.Descriptor{}, err
	}

	tag, desc, err := (&ORASTarget{
		Remote: tmpStore,
		Tag:    t.Tag,
	}).Push(ctx, artifact)

	if err := tar.Directory(f, os.DirFS(tmpDir)); err != nil {
		return "", ocispec.Descriptor{}, err
	}
	return tag, desc, nil
}

func (t *OCIArchiveTarget) String() string {
	return fmt.Sprintf("oci-archive:%s", t.Filename)
}
