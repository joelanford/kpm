package v1_test

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"testing"

	"github.com/containerd/containerd/diff/apply"
	"github.com/containerd/containerd/mount"
	buildv1 "github.com/joelanford/kpm/build/v1"
	"github.com/joelanford/kpm/internal/testutil"
	kpmoci "github.com/joelanford/kpm/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/content/oci"
)

func TestBundle(t *testing.T) {
	type testCase struct {
		name   string
		bundle fs.FS
		assert func(t *testing.T, art kpmoci.Artifact, err error)
	}
	for _, tc := range []testCase{
		{
			name:   "valid bundle",
			bundle: testutil.GenerateBundle("foo", "1.0.0", ""),
			assert: func(t *testing.T, art kpmoci.Artifact, err error) {
				require.NoError(t, err)
				require.NotNil(t, art)

				// Annotations assertions
				actualAnnots, err := art.Annotations()
				assert.NoError(t, err)
				assert.Equal(t, map[string]string{
					"operators.operatorframework.io.bundle.manifests.v1": "manifests/",
					"operators.operatorframework.io.bundle.mediatype.v1": "registry+v1",
					"operators.operatorframework.io.bundle.metadata.v1":  "metadata/",
					"operators.operatorframework.io.bundle.package.v1":   "foo",
				}, actualAnnots)

				expectedBlobDigest := "9c6c2596d2071a35a165c56cedf6d16b25363fda3cdf234e7c592f225298a26b"

				// Config assertions
				actualConfig := art.Config()
				assert.Equal(t, "application/vnd.docker.container.image.v1+json", actualConfig.MediaType())
				actualConfigReader, err := actualConfig.Data()
				require.NoError(t, err)
				defer actualConfigReader.Close()
				actualConfigBytes, err := io.ReadAll(actualConfigReader)
				require.NoError(t, err)
				assert.Equal(t, fmt.Sprintf(`{"architecture":"","os":"linux","config":{"Labels":{"operators.operatorframework.io.bundle.manifests.v1":"manifests/","operators.operatorframework.io.bundle.mediatype.v1":"registry+v1","operators.operatorframework.io.bundle.metadata.v1":"metadata/","operators.operatorframework.io.bundle.package.v1":"foo"}},"rootfs":{"type":"layers","diff_ids":["sha256:%s"]},"history":[{"created_by":"kpm"}]}`, expectedBlobDigest), string(actualConfigBytes))

				// Blob assertions
				actualBlobs := art.Blobs()
				assert.Len(t, actualBlobs, 1)
				assert.Equal(t, "application/vnd.docker.image.rootfs.diff.tar.gzip", actualBlobs[0].MediaType())
				actualBlobReader, err := actualBlobs[0].Data()
				require.NoError(t, err)
				defer actualBlobReader.Close()
				actualDigest := sha256.New()
				_, err = io.Copy(actualDigest, actualBlobReader)
				require.NoError(t, err)
				assert.Equal(t, expectedBlobDigest, fmt.Sprintf("%x", actualDigest.Sum(nil)))

				// Tag assertion
				assert.Equal(t, "v1.0.0-0", art.Tag())

				// Write assertions
				ociDir := t.TempDir()
				ociStore, err := oci.NewWithContext(context.Background(), ociDir)
				require.NoError(t, err)

				imageDesc, err := kpmoci.Push(context.Background(), art, ociStore, kpmoci.PushOptions{})
				require.NoError(t, err)
				imageManifestReader, err := ociStore.Fetch(context.Background(), imageDesc)
				require.NoError(t, err)
				defer imageManifestReader.Close()
				imageManifestData, err := io.ReadAll(imageManifestReader)
				require.NoError(t, err)
				var manifest ocispec.Manifest
				err = json.Unmarshal(imageManifestData, &manifest)
				require.NoError(t, err)

				applier := apply.NewFileSystemApplier(&provider{ociStore})
				m := mount.Mount{
					Type:    "bind",
					Source:  t.TempDir(),
					Target:  t.TempDir(),
					Options: nil,
				}
				applyDesc, err := applier.Apply(context.Background(), manifest.Layers[0], []mount.Mount{m})
				require.NoError(t, err)
				assert.Equal(t, manifest.Layers[0].Digest.String(), applyDesc.Digest.String())
			},
		},
		{
			name:   "explicit release",
			bundle: testutil.GenerateBundle("foo", "1.0.0", "bar"),
			assert: func(t *testing.T, art kpmoci.Artifact, err error) {
				require.NoError(t, err)
				require.NotNil(t, art)
				assert.Equal(t, "v1.0.0-bar", art.Tag())
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			art, err := buildv1.NewBundleBuilder(tc.bundle).BuildArtifact(context.Background())
			tc.assert(t, art, err)
		})
	}
}
