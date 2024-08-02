package v1

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/containers/image/v5/docker/reference"
	kpmoci "github.com/joelanford/kpm/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content/oci"
)

type KPM struct {
	OriginReference reference.Named
	Descriptor      ocispec.Descriptor
	Artifact        kpmoci.Artifact
}

func LoadKPM(ctx context.Context, file string) (*KPM, error) {
	store, err := oci.NewFromTar(ctx, file)
	if err != nil {
		return nil, fmt.Errorf("open kpm file %q: %v", file, err)
	}
	tag := ""
	if err := store.Tags(ctx, "", func(tags []string) error {
		numTags := len(tags)
		if numTags == 0 || numTags > 1 {
			return fmt.Errorf("invalid kpm file: expected exactly one tag, found %d", numTags)
		}
		tag = tags[0]
		return nil
	}); err != nil {
		return nil, err
	}

	desc, err := store.Resolve(ctx, tag)
	if err != nil {
		return nil, fmt.Errorf("resolve tag %q: %v", tag, err)
	}

	originRepository, err := getOriginRepository(ctx, store, desc)
	if err != nil {
		return nil, fmt.Errorf("get origin repository for tag %q: %v", err)
	}

	artifact, err := LoadFromStore(ctx, store, desc)
	if err != nil {
		return nil, fmt.Errorf("load bundle from store: %v", err)
	}

	return &KPM{
		OriginReference: originRepository,
		Descriptor:      desc,
		Artifact:        artifact,
	}, nil
}

func getOriginRepository(ctx context.Context, store *oci.ReadOnlyStore, bundleDesc ocispec.Descriptor) (reference.Named, error) {
	predecessors, err := store.Predecessors(ctx, bundleDesc)
	if err != nil {
		return nil, fmt.Errorf("get predecessors for %q: %v", bundleDesc.Digest, err)
	}

	var originReferenceManifests []ocispec.Manifest

	for _, pred := range predecessors {
		predManifest, err := func() (*ocispec.Manifest, error) {
			predReader, err := store.Fetch(ctx, pred)
			if err != nil {
				return nil, fmt.Errorf("fetch predecessor %q: %v", pred.Digest.String(), err)
			}
			defer predReader.Close()
			dec := json.NewDecoder(predReader)

			if pred.MediaType != ocispec.MediaTypeImageManifest {
				return nil, nil
			}

			var predManifest ocispec.Manifest
			if err := dec.Decode(&predManifest); err != nil {
				return nil, fmt.Errorf("read predecessor manifest: %v", err)
			}
			return &predManifest, nil
		}()
		if err != nil {
			return nil, err
		}
		if predManifest.ArtifactType == ArtifactTypeOriginReference {
			originReferenceManifests = append(originReferenceManifests, *predManifest)
		}
	}

	if len(originReferenceManifests) != 1 {
		return nil, fmt.Errorf("expected exactly one origin reference manifest, found %d", len(originReferenceManifests))
	}

	return reference.ParseNamed(originReferenceManifests[0].Annotations[AnnotationOriginReference])
}
