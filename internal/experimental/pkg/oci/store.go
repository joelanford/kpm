package oci

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
	"oras.land/oras-go/v2/errdef"

	slicesutil "github.com/joelanford/kpm/internal/util/slices"
)

func IgnoreExists(err error) error {
	if errors.Is(err, errdef.ErrAlreadyExists) {
		return nil
	}
	return err
}

func PushArtifact(ctx context.Context, p content.Pusher, a Artifact) (ocispec.Descriptor, error) {
	man := ocispec.Manifest{
		Versioned: specs.Versioned{
			SchemaVersion: 2,
		},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: a.ArtifactType(),
	}

	if aa, ok := a.(Annotated); ok {
		man.Annotations = aa.Annotations()
	}

	var descAnnotations map[string]string
	if da, ok := a.(AnnotatedDescriptor); ok {
		descAnnotations = da.DescriptorAnnotations()
	}

	if r, ok := a.(ArtifactReferrer); ok {
		s := r.Subject()
		desc, err := PushArtifact(ctx, p, s)
		if IgnoreExists(err) != nil {
			return ocispec.Descriptor{}, err
		}
		man.Subject = &desc
	} else if r, ok := a.(ShallowReferrer); ok {
		desc := r.Subject()
		man.Subject = &desc
	}

	if deepArtifact, ok := a.(DeepArtifact); ok {
		cfg := deepArtifact.Config()
		if cfg == nil {
			cfg = EmptyJSONBlob()
		}
		desc, err := PushBlob(ctx, p, cfg)
		if IgnoreExists(err) != nil {
			return ocispec.Descriptor{}, err
		}
		man.Config = desc

		for _, blob := range deepArtifact.Blobs() {
			desc, err := PushBlob(ctx, p, blob)
			if IgnoreExists(err) != nil {
				return ocispec.Descriptor{}, err
			}
			man.Layers = append(man.Layers, desc)
		}
		for _, subartifact := range deepArtifact.SubArtifacts() {
			desc, err := PushArtifact(ctx, p, subartifact)
			if IgnoreExists(err) != nil {
				return ocispec.Descriptor{}, err
			}
			man.Layers = append(man.Layers, desc)
		}
	} else if shallowArtifact, ok := a.(ShallowArtifact); ok {
		man.Config = shallowArtifact.Config()
		man.Layers = append(man.Layers, slicesutil.Collect(shallowArtifact.Blobs())...)
		man.Layers = append(man.Layers, slicesutil.Collect(shallowArtifact.SubArtifacts())...)
	}

	return pushManifest(ctx, p, man, descAnnotations)
}

func pushManifest(ctx context.Context, p content.Pusher, man ocispec.Manifest, descAnnotations map[string]string) (ocispec.Descriptor, error) {
	data, err := json.Marshal(man)
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	desc := content.NewDescriptorFromBytes(man.MediaType, data)
	if descAnnotations != nil {
		desc.Annotations = descAnnotations
	} else {
		desc.Annotations = man.Annotations
	}
	desc.ArtifactType = man.ArtifactType
	return desc, p.Push(ctx, desc, bytes.NewReader(data))
}

func PushBlob(ctx context.Context, p content.Pusher, blob Blob) (ocispec.Descriptor, error) {
	data, err := blob.Data()
	if err != nil {
		return ocispec.Descriptor{}, err
	}
	desc := content.NewDescriptorFromBytes(blob.MediaType(), data)
	if a, ok := blob.(Annotated); ok {
		desc.Annotations = a.Annotations()
	}
	return desc, p.Push(ctx, desc, bytes.NewReader(data))
}
