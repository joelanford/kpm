package catalog

import (
	"context"
	"io/fs"
	"maps"

	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"

	"github.com/operator-framework/operator-registry/pkg/containertools"

	"github.com/joelanford/kpm/internal/fsutil"
	"github.com/joelanford/kpm/internal/kpm"
)

type fbc struct {
	catalog     fs.FS
	cache       fs.FS
	annotations map[string]string
}

func NewFBC(catalogFs fs.FS, cacheFs fs.FS, annotations map[string]string) (Catalog, error) {
	return &fbc{
		catalog:     catalogFs,
		cache:       cacheFs,
		annotations: annotations,
	}, nil
}

func (fb *fbc) Annotations() map[string]string {
	return fb.annotations
}

func (fb *fbc) UnmarshalOCI(ctx context.Context, fetcher content.Fetcher, desc ocispec.Descriptor) error {
	return nil
}

func (fb *fbc) MarshalOCI(ctx context.Context, pusher content.Pusher) (ocispec.Descriptor, error) {
	annotations := make(map[string]string, len(fb.annotations))
	maps.Copy(annotations, fb.annotations)
	annotations[containertools.ConfigsLocationLabel] = "/configs"

	configs, err := fsutil.Prefix("configs", fb.catalog)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	layers := []fs.FS{configs}
	if fb.cache != nil {
		tmpCache, err := fsutil.Prefix("tmp/cache", fb.cache)
		if err != nil {
			return ocispec.Descriptor{}, err
		}
		layers = append(layers, tmpCache)
	}
	return kpm.MarshalOCIManifest(ctx, pusher, layers, annotations)
}
