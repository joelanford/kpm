package kpm

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"

	"github.com/blang/semver/v4"
	"github.com/containers/image/v5/docker/reference"
	"github.com/opencontainers/go-digest"
	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/content/oci"

	"github.com/joelanford/kpm/internal/pkg/util/tar"
)

const (
	AnnotationName    = "io.operatorframework.kpm.name"
	AnnotationVersion = "io.operatorframework.kpm.version"
	AnnotationRelease = "io.operatorframework.kpm.release"
)

type ID struct {
	Name    string
	Version semver.Version
	Release string
}

func PushManifest(ctx context.Context, pusher content.Pusher, layers []fs.FS, labels, annotations map[string]string) (ocispec.Descriptor, error) {
	var (
		diffIDs    []digest.Digest
		layerDescs []ocispec.Descriptor
	)

	for i, layerFsys := range layers {
		l, err := buildLayer(ocispec.MediaTypeImageLayerGzip, layerFsys)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to build layer[%d]: %w", i, err)
		}
		if err := pushIfNotExists(context.Background(), pusher, l.descriptor, bytes.NewReader(l.data)); err != nil {
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
	if err := pushIfNotExists(ctx, pusher, configDesc, bytes.NewReader(configData)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push config data: %w", err)
	}

	manifestDesc, manifestData, err := generateManifest(configDesc, annotations, layerDescs...)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to generate manifest data: %w", err)
	}
	if err := pushIfNotExists(ctx, pusher, manifestDesc, bytes.NewReader(manifestData)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push manifest data: %w", err)
	}
	return manifestDesc, nil
}

func WriteImageManifest(w io.Writer, name reference.NamedTagged, layers []fs.FS, labels map[string]string, annotations map[string]string) (ocispec.Descriptor, error) {
	// Create a temporary directory to build the OCI directory
	tmpDir, err := os.MkdirTemp("", "kpm-build-")
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	kpmStore, err := oci.NewWithContext(context.Background(), tmpDir)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to create kpm store: %w", err)
	}

	manifestDesc, err := PushManifest(context.Background(), kpmStore, layers, labels, annotations)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push manifest: %w", err)
	}

	if name != nil {
		if err := kpmStore.Tag(context.Background(), manifestDesc, name.String()); err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to tag kpm: %w", err)
		}
	}

	if err := tar.Directory(w, os.DirFS(tmpDir)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to write OCI directory: %w", err)
	}

	return manifestDesc, nil
}

type layer struct {
	descriptor ocispec.Descriptor
	diffID     digest.Digest
	data       []byte
}

func buildLayer(mediaType string, fsys fs.FS) (*layer, error) {
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

func WriteHelmChartManifest(w io.Writer, name reference.NamedTagged, chartRoot fs.FS, provenanceData []byte, labels map[string]string) (ocispec.Descriptor, error) {
	// Create a temporary directory to build the OCI directory
	tmpDir, err := os.MkdirTemp("", "kpm-build-")
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	kpmStore, err := oci.NewWithContext(context.Background(), tmpDir)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to create kpm store: %w", err)
	}

	ch, err := loadHelmChartFS(chartRoot)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to load helm chart: %w", err)
	}
	chartLayer, err := buildLayer(registry.ChartLayerMediaType, chartRoot)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to build chart layer: %w", err)
	}
	if err := pushIfNotExists(context.Background(), kpmStore, chartLayer.descriptor, bytes.NewReader(chartLayer.data)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push chart layer: %w", err)
	}

	layerDescs := []ocispec.Descriptor{chartLayer.descriptor}
	if provenanceData != nil {
		provDesc, err := oras.PushBytes(context.Background(), kpmStore, registry.ProvLayerMediaType, provenanceData)
		if err != nil {
			return ocispec.Descriptor{}, fmt.Errorf("failed to push provenance data: %w", err)
		}
		layerDescs = append(layerDescs, provDesc)
	}

	configData, err := json.Marshal(ch.Metadata)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to marshal config data: %w", err)
	}

	configDesc, err := oras.PushBytes(context.Background(), kpmStore, registry.ConfigMediaType, configData)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push config data: %w", err)
	}

	annotations := maps.Clone(labels)
	annotations[ocispec.AnnotationRefName] = name.String()

	manifestDesc, manifestData, err := generateManifest(configDesc, annotations, layerDescs...)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to generate manifest data: %w", err)
	}
	if err := pushIfNotExists(context.Background(), kpmStore, manifestDesc, bytes.NewReader(manifestData)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push manifest data: %w", err)
	}

	if err := kpmStore.Tag(context.Background(), manifestDesc, name.Tag()); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to tag kpm: %w", err)
	}

	if err := tar.Directory(w, os.DirFS(tmpDir)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to write OCI directory: %w", err)
	}

	return manifestDesc, nil
}

func loadHelmChartFS(chartRoot fs.FS) (*chart.Chart, error) {
	pr, pw := io.Pipe()
	go func() {
		pw.CloseWithError(tar.Directory(pw, chartRoot))
	}()

	ch, err := loader.LoadArchive(pr)
	if err != nil {
		return nil, fmt.Errorf("failed to load chart: %w", err)
	}
	if err := ch.Validate(); err != nil {
		return nil, fmt.Errorf("failed to validate chart: %w", err)
	}
	return ch, nil
}

func generateManifest(config ocispec.Descriptor, annotations map[string]string, layers ...ocispec.Descriptor) (ocispec.Descriptor, []byte, error) {
	manifest := ocispec.Manifest{
		Versioned:   specs.Versioned{SchemaVersion: 2},
		MediaType:   ocispec.MediaTypeImageManifest,
		Annotations: annotations,
		Config:      config,
		Layers:      layers,
	}

	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, nil, fmt.Errorf("failed to marshal manifest: %w", err)
	}
	manifestDesc := content.NewDescriptorFromBytes(manifest.MediaType, manifestJSON)
	manifestDesc.Annotations = annotations
	return manifestDesc, manifestJSON, nil
}

func pushIfNotExists(ctx context.Context, pusher content.Pusher, desc ocispec.Descriptor, r io.Reader) error {
	if ros, ok := pusher.(content.ReadOnlyStorage); ok {
		exists, err := ros.Exists(ctx, desc)
		if err != nil {
			return fmt.Errorf("failed to check existence: %s: %s: %w", desc.Digest.String(), desc.MediaType, err)
		}
		if exists {
			return nil
		}
	}
	return pusher.Push(ctx, desc, r)
}
