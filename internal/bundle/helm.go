package bundle

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/blang/semver/v4"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/registry"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"

	"github.com/joelanford/kpm/internal/ociutil"
)

func NewHelm(chartArchivePath string, annotations map[string]string) (Bundle, error) {
	chartData, err := os.ReadFile(chartArchivePath)
	if err != nil {
		return nil, err
	}

	var provData []byte
	provFilePath := fmt.Sprintf("%s.prov", chartArchivePath)
	if _, err := os.Stat(provFilePath); err == nil {
		provData, err = os.ReadFile(provFilePath)
		if err != nil {
			return nil, err
		}
	}

	chrt, err := loader.LoadArchive(bytes.NewReader(chartData))
	if err != nil {
		return nil, err
	}
	if chrt.Metadata == nil {
		return nil, fmt.Errorf("chart metadata is empty")
	}
	if chrt.Metadata.Name == "" {
		return nil, fmt.Errorf("chart name is empty")
	}
	if chrt.Metadata.Version == "" {
		return nil, fmt.Errorf("chart version is empty")
	}
	v, err := semver.Parse(chrt.Metadata.Version)
	if err != nil {
		return nil, err
	}

	return &helm{
		chartData:   chartData,
		provData:    provData,
		metadata:    *chrt.Metadata,
		version:     v,
		annotations: annotations,
	}, nil
}

type helm struct {
	chartData   []byte
	provData    []byte
	metadata    chart.Metadata
	version     semver.Version
	annotations map[string]string
}

func (h *helm) PackageName() string {
	return h.metadata.Name
}

func (h *helm) Version() semver.Version {
	return h.version
}

func (h *helm) Annotations() map[string]string {
	return h.annotations
}

func (h *helm) writeOCILayers(ctx context.Context, pusher content.Pusher) ([]ocispec.Descriptor, error) {
	chartDesc, err := oras.PushBytes(ctx, pusher, registry.ChartLayerMediaType, h.chartData)
	if err != nil {
		return nil, fmt.Errorf("failed to push chart layer: %w", err)
	}

	layers := []ocispec.Descriptor{chartDesc}
	if len(h.provData) > 0 {
		var provDesc ocispec.Descriptor
		provDesc, err = oras.PushBytes(ctx, pusher, registry.ProvLayerMediaType, h.provData)
		if err := pusher.Push(ctx, provDesc, bytes.NewReader(h.chartData)); err != nil {
			return nil, fmt.Errorf("failed to push provenance layer: %w", err)
		}
		layers = append(layers, provDesc)
	}
	return layers, nil
}

func (h *helm) writeOCIConfig(ctx context.Context, pusher content.Pusher) (ocispec.Descriptor, error) {
	configData, err := json.Marshal(h.metadata)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to marshal config data: %w", err)
	}
	return oras.PushBytes(ctx, pusher, registry.ConfigMediaType, configData)
}

func (h *helm) MarshalOCI(ctx context.Context, pusher content.Pusher) (ocispec.Descriptor, error) {
	config, err := h.writeOCIConfig(ctx, pusher)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to write helm config: %w", err)
	}

	layers, err := h.writeOCILayers(ctx, pusher)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to write helm layers: %w", err)
	}

	return ociutil.PushManifest(ctx, pusher, config, layers, h.annotations)
}
func (h *helm) UnmarshalOCI(ctx context.Context, fetcher content.Fetcher, desc ocispec.Descriptor) error {
	manifestData, err := content.FetchAll(ctx, fetcher, desc)
	if err != nil {
		return err
	}
	var manifest ocispec.Manifest
	if err := json.Unmarshal(manifestData, &manifest); err != nil {
		return err
	}
	for _, l := range manifest.Layers {
		switch l.MediaType {
		case registry.ChartLayerMediaType:
			h.chartData, err = content.FetchAll(ctx, fetcher, l)
			if err != nil {
				return err
			}
		case registry.ProvLayerMediaType:
			h.provData, err = content.FetchAll(ctx, fetcher, l)
			if err != nil {
				return err
			}
		}
	}
	configData, err := content.FetchAll(ctx, fetcher, manifest.Config)
	if err != nil {
		return err
	}

	chrt, err := loader.LoadArchive(bytes.NewReader(h.chartData))
	if err != nil {
		return err
	}

	chartMetadata, err := json.Marshal(chrt.Metadata)
	if err != nil {
		return err
	}
	if !bytes.Equal(configData, chartMetadata) {
		return fmt.Errorf("chart config data from %q does not match metadata embedded in chart layer %q", registry.ConfigMediaType, registry.ChartLayerMediaType)
	}

	h.metadata = *chrt.Metadata
	h.annotations = manifest.Annotations
	h.version, err = semver.Parse(h.metadata.Version)
	if err != nil {
		return err
	}
	return nil
}
