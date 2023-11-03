package oci_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"testing"

	"github.com/opencontainers/image-spec/specs-go"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/content/memory"

	v1 "github.com/joelanford/kpm/api/v1"
	"github.com/joelanford/kpm/internal/testutil"
	"github.com/joelanford/kpm/oci"
)

type testCase struct {
	name        string
	pushOptions oci.PushOptions
}

func TestPush_Package(t *testing.T) {
	for _, tc := range []testCase{
		{
			name:        "no progress",
			pushOptions: oci.PushOptions{},
		},
		{
			name: "progress byte buffer",
			pushOptions: oci.PushOptions{
				ProgressWriter: &bytes.Buffer{},
			},
		},
		{
			name: "progress stdout",
			pushOptions: oci.PushOptions{
				ProgressWriter: os.Stdout,
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pushTest(t, tc)
		})
	}
}

func pushTest(t *testing.T, tc testCase) {
	pkg := testutil.GetSimplePackage(t)
	pkgAnnotations, err := pkg.Annotations()
	require.NoError(t, err)

	store := memory.New()
	pkgDesc, err := oci.Push(context.Background(), pkg, store, tc.pushOptions)

	require.NoError(t, err)
	assert.Equal(t, ocispec.Descriptor{
		MediaType:    ocispec.MediaTypeImageManifest,
		Digest:       "sha256:e232c424c78f2b5019e55137d730a7c99cc9f8a2aaf2531e849ecad5987355cc",
		Size:         1022,
		ArtifactType: v1.ArtifactTypePackage,
	}, pkgDesc)

	pkgReader, err := store.Fetch(context.Background(), pkgDesc)
	require.NoError(t, err)

	var pkgManifest ocispec.Manifest
	require.NoError(t, json.NewDecoder(pkgReader).Decode(&pkgManifest))
	assert.NoError(t, pkgReader.Close())
	assert.Equal(t, ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: v1.ArtifactTypePackage,
		Config: ocispec.Descriptor{
			MediaType: v1.MediaTypePackageConfig,
			Digest:    "sha256:f309ddfe6d9b8724cbbb90640622ff71ac156b4116590c3c4ae3b9ee65c450d5",
			Size:      266,
		},
		Layers: []ocispec.Descriptor{
			{MediaType: "image/svg+xml", Digest: "sha256:346608d58ff1b847358ca46e941a0aba40be7f37ec14286a20abe24a5edf1176", Size: 129},
			{MediaType: "application/vnd.oci.image.manifest.v1+json", Digest: "sha256:57a8d047f26b18015c527ab6aa01bcd6d0135f61cd1f33c6315b248bcbd80b34", Size: 527, ArtifactType: "application/vnd.io.kpm.bundle.v1+json"},
			{MediaType: "application/vnd.oci.image.manifest.v1+json", Digest: "sha256:39c2dbd914eb71d849ab37402867b3d44a2447a69105d3a5677bfd5e570ab1a5", Size: 529, ArtifactType: "application/vnd.io.kpm.bundle.v1+json"},
		},
		Annotations: pkgAnnotations,
	}, pkgManifest)

	// Verify icon
	iconDesc := pkgManifest.Layers[0]
	iconReader, err := store.Fetch(context.Background(), iconDesc)
	require.NoError(t, err)

	iconData, err := io.ReadAll(iconReader)
	require.NoError(t, err)
	assert.NoError(t, iconReader.Close())

	assert.Equal(t, pkg.Icon.MediaType, iconDesc.MediaType)
	assert.Equal(t, pkg.Icon.Data, iconData)

	// Verify bundle 1
	b1Annotations, err := pkg.Bundles[0].Annotations()
	require.NoError(t, err)

	bundle1Desc := pkgManifest.Layers[1]
	b1Reader, err := store.Fetch(context.Background(), bundle1Desc)
	require.NoError(t, err)

	var b1Manifest ocispec.Manifest
	require.NoError(t, json.NewDecoder(b1Reader).Decode(&b1Manifest))
	assert.NoError(t, b1Reader.Close())
	assert.Equal(t, ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: v1.ArtifactTypeBundle,
		Config: ocispec.Descriptor{
			MediaType: v1.MediaTypeBundleConfig,
			Digest:    "sha256:c5e501f40bd59db9ac6f59a8bebec22a992ea6f3e07fb9bf3f1a68e185e4c789",
			Size:      176,
		},
		Layers: []ocispec.Descriptor{
			{MediaType: "application/yaml", Digest: "sha256:b301bcab0b77b8feb1a1d98b39233b26f1742acd2fcb51a166bfd20ed8891e64", Size: 124},
		},
		Annotations: b1Annotations,
	}, b1Manifest)

	b1ConfigReader, err := store.Fetch(context.Background(), b1Manifest.Config)
	require.NoError(t, err)
	var b1Config v1.BundleConfig
	require.NoError(t, json.NewDecoder(b1ConfigReader).Decode(&b1Config))
	assert.NoError(t, b1ConfigReader.Close())
	assert.Equal(t, v1.BundleConfig{
		Name:      "foo",
		Version:   "1.0.0",
		Release:   "1",
		Provides:  []string{"package(foo=1.0.0)"},
		Requires:  []string{"package(bar)", "api(widgets.acme.io/v1alpha1)"},
		Conflicts: []string{"package(foo-legacy)"},
	}, b1Config)

	// Verify bundle 2
	b2Annotations, err := pkg.Bundles[1].Annotations()
	require.NoError(t, err)

	bundle2Desc := pkgManifest.Layers[2]
	b2Reader, err := store.Fetch(context.Background(), bundle2Desc)
	require.NoError(t, err)

	var b2Manifest ocispec.Manifest
	require.NoError(t, json.NewDecoder(b2Reader).Decode(&b2Manifest))
	assert.NoError(t, b2Reader.Close())
	assert.Equal(t, ocispec.Manifest{
		Versioned:    specs.Versioned{SchemaVersion: 2},
		MediaType:    ocispec.MediaTypeImageManifest,
		ArtifactType: v1.ArtifactTypeBundle,
		Config: ocispec.Descriptor{
			MediaType: v1.MediaTypeBundleConfig,
			Digest:    "sha256:61350b2b04a8d905ea4742edbd5eb957ee91b50d4c54fdaebf71cef96d4b1db8",
			Size:      176,
		},
		Layers: []ocispec.Descriptor{
			{MediaType: "application/yaml", Digest: "sha256:b301bcab0b77b8feb1a1d98b39233b26f1742acd2fcb51a166bfd20ed8891e64", Size: 124},
		},
		Annotations: b2Annotations,
	}, b2Manifest)

	b2ConfigReader, err := store.Fetch(context.Background(), b2Manifest.Config)
	require.NoError(t, err)
	var b2Config v1.BundleConfig
	require.NoError(t, json.NewDecoder(b2ConfigReader).Decode(&b2Config))
	assert.NoError(t, b1ConfigReader.Close())
	assert.Equal(t, v1.BundleConfig{
		Name:      "foo",
		Version:   "1.0.0",
		Release:   "2",
		Provides:  []string{"package(foo=1.0.0)"},
		Requires:  []string{"package(bar)", "api(widgets.acme.io/v1alpha1)"},
		Conflicts: []string{"package(foo-legacy)"},
	}, b2Config)
}
