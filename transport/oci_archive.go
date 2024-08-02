package transport

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/containers/image/v5/docker/reference"
	apiv1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/tar"
	kpmoci "github.com/joelanford/kpm/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/oci"
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
	Filename        string
	OriginReference reference.Named
	Tag             string
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

	orasTarget := &ORASTarget{
		Remote: tmpStore,
		Tag:    t.Tag,
	}
	tag, desc, err := orasTarget.Push(ctx, artifact)
	if err != nil {
		return "", ocispec.Descriptor{}, err
	}
	if t.OriginReference != nil {
		originRef := apiv1.NewOriginReference(desc, t.OriginReference)
		_, _, err := orasTarget.Push(ctx, originRef)
		if err != nil {
			return "", ocispec.Descriptor{}, err
		}
	}

	if err := tar.Directory(f, os.DirFS(tmpDir)); err != nil {
		return "", ocispec.Descriptor{}, err
	}
	return tag, desc, nil
}

func (t *OCIArchiveTarget) String() string {
	return fmt.Sprintf("oci-archive:%s", t.Filename)
}
