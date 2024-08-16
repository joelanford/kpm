package v1

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	kpmoci "github.com/joelanford/kpm/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/oci"
)

type OCIManifest struct {
	configData  []byte
	blobData    []byte
	annotations map[string]string
	tag         string
}

func NewOCIManifest(tag string, configData, blobData []byte, annotations map[string]string) *OCIManifest {
	return &OCIManifest{
		tag:         tag,
		configData:  configData,
		blobData:    blobData,
		annotations: annotations,
	}
}

func (l OCIManifest) MediaType() string {
	return ocispec.MediaTypeImageManifest
}

func (l OCIManifest) ArtifactType() string {
	return ""
}

func (l OCIManifest) Config() kpmoci.Blob {
	return kpmoci.BlobFromBytes(ocispec.MediaTypeImageConfig, l.configData)
}

func (l OCIManifest) Annotations() (map[string]string, error) {
	return l.annotations, nil
}

func (l OCIManifest) SubArtifacts() []kpmoci.Artifact {
	return nil
}

func (l OCIManifest) Blobs() []kpmoci.Blob {
	return []kpmoci.Blob{kpmoci.BlobFromBytes(ocispec.MediaTypeImageLayerGzip, l.blobData)}
}

func (l OCIManifest) Subject() *ocispec.Descriptor {
	return nil
}

func (l OCIManifest) Tag() string {
	return l.tag
}

func LoadFromStore(ctx context.Context, store *oci.ReadOnlyStore, desc ocispec.Descriptor) (*OCIManifest, error) {
	if desc.MediaType != ocispec.MediaTypeImageManifest {
		return nil, fmt.Errorf("unsupported media type %q, expecting %q", desc.MediaType, ocispec.MediaTypeImageManifest)
	}

	var tagsForDesc []string
	if err := store.Tags(ctx, "", func(tags []string) error {
		for _, t := range tags {
			tagDesc, err := store.Resolve(ctx, t)
			if err != nil {
				return fmt.Errorf("resolve tag %q: %w", t, err)
			}
			if tagDesc.Digest.String() == desc.Digest.String() {
				tagsForDesc = append(tagsForDesc, t)
			}
		}
		return nil
	}); err != nil {
		return nil, fmt.Errorf("lookup tag for digest %q: %w", desc.Digest, err)
	}

	if len(tagsForDesc) > 1 {
		return nil, fmt.Errorf("found multiple tags for digest %q: %v", desc.Digest, tagsForDesc)
	}
	if len(tagsForDesc) == 0 {
		tagsForDesc = append(tagsForDesc, "")
	}

	manifestReader, err := store.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("fetching descriptor %v: %w", desc, err)
	}
	defer manifestReader.Close()

	dec := json.NewDecoder(manifestReader)
	var manifest ocispec.Manifest
	if err := dec.Decode(&manifest); err != nil {
		return nil, fmt.Errorf("decoding manifest: %w", err)
	}

	if len(manifest.Layers) != 1 {
		return nil, fmt.Errorf("found multiple layers, expecting exactly 1")
	}

	configData, err := readAll(ctx, store, manifest.Config)
	if err != nil {
		return nil, fmt.Errorf("read config data: %w", err)
	}
	blobData, err := readAll(ctx, store, manifest.Layers[0])
	if err != nil {
		return nil, fmt.Errorf("read blob data: %w", err)
	}

	// recover annotations from docker manifest config layer
	var config ocispec.Image
	if err := json.Unmarshal(configData, &config); err != nil {
		return nil, fmt.Errorf("unmarshal config data: %w", err)
	}

	return &OCIManifest{
		configData:  configData,
		blobData:    blobData,
		annotations: config.Config.Labels,
		tag:         tagsForDesc[0],
	}, nil
}

func readAll(ctx context.Context, store *oci.ReadOnlyStore, desc ocispec.Descriptor) ([]byte, error) {
	r, err := store.Fetch(ctx, desc)
	if err != nil {
		return nil, fmt.Errorf("fetching descriptor %v: %w", desc, err)
	}
	defer r.Close()
	return io.ReadAll(r)
}
