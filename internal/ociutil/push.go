package ociutil

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"oras.land/oras-go/v2/content"
)

func PushIfNotExists(ctx context.Context, pusher content.Pusher, desc ocispec.Descriptor, r io.Reader) error {
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

func PushManifest(ctx context.Context, p content.Pusher, configDesc ocispec.Descriptor, layerDescs []ocispec.Descriptor, annotations map[string]string) (ocispec.Descriptor, error) {
	manifest := ocispec.Manifest{
		Versioned:   specs.Versioned{SchemaVersion: 2},
		MediaType:   ocispec.MediaTypeImageManifest,
		Annotations: annotations,
		Config:      configDesc,
		Layers:      layerDescs,
	}
	manifestData, err := json.Marshal(manifest)
	if err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to marshal manifest: %w", err)
	}
	manifestDesc := content.NewDescriptorFromBytes(manifest.MediaType, manifestData)
	if err := PushIfNotExists(ctx, p, manifestDesc, bytes.NewReader(manifestData)); err != nil {
		return ocispec.Descriptor{}, fmt.Errorf("failed to push manifest: %w", err)
	}
	return manifestDesc, nil
}
