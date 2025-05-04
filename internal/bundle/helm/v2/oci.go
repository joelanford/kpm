package v2

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/registry"
	"oras.land/oras-go/v2"
	"oras.land/oras-go/v2/content"

	bundlev1alpha1 "github.com/joelanford/kpm/internal/api/bundle/v1alpha1"
)

func (p *Package) ID() bundlev1alpha1.ID {
	return p.id
}

func (p *Package) MarshalOCI(ctx context.Context, pusher content.Pusher) (ocispec.Descriptor, error) {
	config, layers, err := p.pushConfigAndLayers(ctx, pusher)
	if err != nil {
		return ocispec.Descriptor{}, err
	}

	manifest := ocispec.Manifest{
		Versioned:   specs.Versioned{SchemaVersion: 2},
		MediaType:   ocispec.MediaTypeImageManifest,
		Config:      config,
		Layers:      layers,
		Annotations: p.generateOCIAnnotations(),
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	return oras.PushBytes(ctx, pusher, ocispec.MediaTypeImageManifest, manifestData)
}

func (p *Package) pushConfigAndLayers(ctx context.Context, pusher content.Pusher) (ocispec.Descriptor, []ocispec.Descriptor, error) {
	configData, err := json.Marshal(p.chrt.Metadata)
	if err != nil {
		return ocispec.Descriptor{}, nil, fmt.Errorf("failed to marshal chart metadata: %w", err)
	}
	configDesc, err := oras.PushBytes(ctx, pusher, registry.ConfigMediaType, configData)
	if err != nil {
		return ocispec.Descriptor{}, nil, fmt.Errorf("failed to push config data: %w", err)
	}

	var layerDescs []ocispec.Descriptor
	chartDesc, err := oras.PushBytes(ctx, pusher, registry.ChartLayerMediaType, p.archiveData)
	if err != nil {
		return ocispec.Descriptor{}, nil, fmt.Errorf("failed to push chart data: %w", err)
	}
	layerDescs = append(layerDescs, chartDesc)

	if len(p.provenanceData) > 0 {
		provDesc, err := oras.PushBytes(ctx, pusher, registry.ProvLayerMediaType, p.provenanceData)
		if err != nil {
			return ocispec.Descriptor{}, nil, fmt.Errorf("failed to push provenance data: %w", err)
		}
		layerDescs = append(layerDescs, provDesc)
	}
	return configDesc, layerDescs, nil
}

func (p *Package) generateOCIAnnotations() map[string]string {
	meta := p.chrt.Metadata

	annotations := maps.Clone(meta.Annotations)
	if annotations == nil {
		annotations = make(map[string]string, 5)
	}

	annotations[ocispec.AnnotationDescription] = meta.Description
	annotations[ocispec.AnnotationTitle] = meta.Name
	annotations[ocispec.AnnotationVersion] = meta.Version
	annotations[ocispec.AnnotationURL] = meta.Home
	annotations[ocispec.AnnotationAuthors] = maintainersToString(meta.Maintainers)

	if len(meta.Sources) > 0 {
		annotations[ocispec.AnnotationSource] = meta.Sources[0]
	}

	// delete map entries that have empty values
	maps.DeleteFunc(annotations, func(k string, v string) bool {
		return v == ""
	})

	return annotations
}

func maintainersToString(chartMaintainers []*chart.Maintainer) string {
	var maintainers []string
	for _, maintainer := range chartMaintainers {
		if maintainer == nil {
			continue
		}
		var maintainerStr strings.Builder
		if len(maintainer.Name) > 0 {
			maintainerStr.WriteString(maintainer.Name)
		}

		if len(maintainer.Email) > 0 {
			maintainerStr.WriteString(" (")
			maintainerStr.WriteString(maintainer.Email)
			maintainerStr.WriteString(")")
		}
		maintainers = append(maintainers, maintainerStr.String())
	}
	return strings.Join(maintainers, ", ")
}
