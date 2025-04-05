package kpm

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"

	"github.com/containers/image/v5/docker/reference"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"

	"github.com/joelanford/kpm/internal/ociutil"
	"github.com/joelanford/kpm/internal/tar"
)

type OCIMarshaler interface {
	MarshalOCI(context.Context, content.Pusher) (ocispec.Descriptor, error)
}

type OCIUnmarshaler interface {
	UnmarshalOCI(context.Context, content.Fetcher, ocispec.Descriptor) error
}

func BuildFile(ctx context.Context, fileName string, m OCIMarshaler, imageReference string) (reference.NamedTagged, ocispec.Descriptor, error) {
	file, err := os.Create(fileName)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to create output file: %v", err)
	}
	defer file.Close()

	tagRef, desc, err := BuildWriter(ctx, file, m, imageReference)
	if err != nil {
		return nil, ocispec.Descriptor{}, errors.Join(err, os.Remove(fileName))
	}

	return tagRef, desc, nil
}

// BuildWriter writes a bundle to a writer
func BuildWriter(ctx context.Context, w io.Writer, m OCIMarshaler, imageReference string) (reference.NamedTagged, ocispec.Descriptor, error) {
	tagRef, err := ociutil.ParseNamedTagged(imageReference)
	if err != nil {
		return nil, ocispec.Descriptor{}, err
	}

	tmpDir, err := os.MkdirTemp("", "kpm-build-")
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to create temporary directory: %v", err)
	}
	defer os.RemoveAll(tmpDir)
	tmpLayout, err := oci.NewWithContext(ctx, tmpDir)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to create temporary layout: %v", err)
	}
	desc, err := m.MarshalOCI(ctx, tmpLayout)
	if err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to write kpm file: %v", err)
	}

	if err := tmpLayout.Tag(ctx, desc, tagRef.String()); err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to tag kpm: %w", err)
	}

	if err := tar.Directory(w, os.DirFS(tmpDir)); err != nil {
		return nil, ocispec.Descriptor{}, fmt.Errorf("failed to write OCI directory: %w", err)
	}

	return tagRef, desc, nil
}

func MarshalOCIManifest(ctx context.Context, p content.Pusher, layers []fs.FS, labels map[string]string) (ocispec.Descriptor, error) {
	var (
		diffIDs    []digest.Digest
		layerDescs []ocispec.Descriptor
	)

	for i, layerFsys := range layers {
		l, err := buildImageLayer(layerFsys)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to build layer[%d]: %w", i, err)
		}
		if err := ociutil.PushIfNotExists(ctx, p, l.descriptor, bytes.NewReader(l.data)); err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to push layer data: %w", err)
		}

		diffIDs = append(diffIDs, l.diffID)
		layerDescs = append(layerDescs, l.descriptor)
	}

	configData, err := json.Marshal(ocispec.Image{
		Config: ocispec.ImageConfig{
			Labels: labels,
		},
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: diffIDs,
		},
		Platform: ocispec.Platform{
			OS: "linux",
		},
	})
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to marshal config data: %w", err)
	}

	configDesc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageConfig,
		Digest:    digest.FromBytes(configData),
		Size:      int64(len(configData)),
	}
	if err := ociutil.PushIfNotExists(ctx, p, configDesc, bytes.NewReader(configData)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push config data: %w", err)
	}

	manifestDesc, err := ociutil.PushManifest(ctx, p, configDesc, layerDescs, labels)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push manifest: %w", err)
	}
	return manifestDesc, nil
}

type layer struct {
	descriptor ocispec.Descriptor
	diffID     digest.Digest
	data       []byte
}

func buildImageLayer(fsys fs.FS) (*layer, error) {
	// fsys -> [tarredWriter -> tarredReader] -> gzipWriter -> [gzippedWriter -> gzippedReader] -> layerData
	//      -> diffIDWriter                                 -> layerDigestWriter

	tarredReader, tarredWriter := io.Pipe()
	diffIDWriter := sha256.New()
	tarMultiWriter := io.MultiWriter(diffIDWriter, tarredWriter)
	go func() {
		tarredWriter.CloseWithError(tar.Directory(tarMultiWriter, fsys))
	}()

	gzippedReader, gzippedWriter := io.Pipe()
	layerDigestWriter := sha256.New()
	gzipMultiWriter := io.MultiWriter(gzippedWriter, layerDigestWriter)
	gzipWriter := gzip.NewWriter(gzipMultiWriter)
	go func() {
		_, err := io.Copy(gzipWriter, tarredReader)
		gzipWriter.Close()
		gzippedWriter.CloseWithError(err)
	}()

	data, err := io.ReadAll(gzippedReader)
	if err != nil {
		return nil, fmt.Errorf("failed to read layer data: %w", err)
	}

	desc := ocispec.Descriptor{
		MediaType: ocispec.MediaTypeImageLayerGzip,
		Digest:    digest.NewDigestFromEncoded(digest.SHA256, fmt.Sprintf("%x", layerDigestWriter.Sum(nil))),
		Size:      int64(len(data)),
	}

	return &layer{
		descriptor: desc,
		diffID:     digest.NewDigestFromEncoded(digest.SHA256, fmt.Sprintf("%x", diffIDWriter.Sum(nil))),
		data:       data,
	}, nil
}
