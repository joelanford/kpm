package v1_test

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"testing"

	"github.com/containerd/containerd/content"
	"github.com/containerd/containerd/diff/apply"
	"github.com/containerd/containerd/mount"
	"github.com/joelanford/kpm/action"
	buildv1 "github.com/joelanford/kpm/build/v1"
	"github.com/joelanford/kpm/internal/testutil"
	kpmoci "github.com/joelanford/kpm/oci"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/content/oci"
)

func TestCatalog(t *testing.T) {
	type testCase struct {
		name    string
		catalog fs.FS
		tag     string
		assert  func(t *testing.T, art kpmoci.Artifact, err error)
	}
	for _, tc := range []testCase{
		{
			name: "valid catalog",
			catalog: mustGenCatalog(t, "localregistry/foo",
				testutil.GenerateBundle("foo", "1.0.0", ""),
				testutil.GenerateBundle("foo", "1.0.0", "1"),
				testutil.GenerateBundle("foo", "1.0.1", ""),
				testutil.GenerateBundle("foo", "1.0.2", ""),
				testutil.GenerateBundle("foo", "1.0.2", "1"),
				testutil.GenerateBundle("foo", "1.0.2", "2"),
				testutil.GenerateBundle("foo", "1.0.2", "3"),
				testutil.GenerateBundle("foo", "1.0.2", "4"),
				testutil.GenerateBundle("foo", "1.0.3", ""),
				testutil.GenerateBundle("foo", "1.1.0", ""),
				testutil.GenerateBundle("foo", "1.1.0", "1"),
				testutil.GenerateBundle("foo", "1.1.0", "2"),
				testutil.GenerateBundle("foo", "1.1.0", "3"),
				testutil.GenerateBundle("foo", "1.1.0", "4"),
				testutil.GenerateBundle("foo", "2.0.0", ""),
				testutil.GenerateBundle("foo", "2.1.0", ""),
				testutil.GenerateBundle("foo", "2.2.0", ""),
				testutil.GenerateBundle("foo", "2.2.0", "1"),
				testutil.GenerateBundle("foo", "2.2.0", "2"),
				testutil.GenerateBundle("foo", "2.3.0", ""),
				testutil.GenerateBundle("foo", "2.3.0", "1"),
			),
			assert: func(t *testing.T, art kpmoci.Artifact, err error) {
				require.NoError(t, err)
				require.NotNil(t, art)

				// Annotations assertions
				actualAnnots, err := art.Annotations()
				assert.NoError(t, err)
				assert.Equal(t, map[string]string{
					"operators.operatorframework.io.index.cache.v1":   "/tmp/cache",
					"operators.operatorframework.io.index.configs.v1": "/configs",
				}, actualAnnots)

				expectedBlobDigest := "8d0f29630db3a4ae2b7d9b73a7d20a0ddb1e8541af5f2de67b8d73ae8c1ce49b"

				// Config assertions
				actualConfig := art.Config()
				assert.Equal(t, "application/vnd.docker.container.image.v1+json", actualConfig.MediaType())
				actualConfigReader, err := actualConfig.Data()
				require.NoError(t, err)
				defer actualConfigReader.Close()
				actualConfigBytes, err := io.ReadAll(actualConfigReader)
				require.NoError(t, err)
				assert.Equal(t, fmt.Sprintf(`{"architecture":"","os":"linux","config":{"Labels":{"operators.operatorframework.io.index.cache.v1":"/tmp/cache","operators.operatorframework.io.index.configs.v1":"/configs"}},"rootfs":{"type":"layers","diff_ids":["sha256:%s"]},"history":[{"created_by":"kpm"}]}`, expectedBlobDigest), string(actualConfigBytes))

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
				assert.Equal(t, "latest", art.Tag())

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
	} {
		t.Run(tc.name, func(t *testing.T) {
			var opts []buildv1.BuildOption
			art, err := buildv1.NewCatalogBuilder(tc.catalog, opts...).BuildArtifact(context.Background())
			tc.assert(t, art, err)
		})
	}
}

func mustGenCatalog(t *testing.T, repo string, bundles ...fs.FS) fs.FS {
	t.Helper()
	var artifacts []kpmoci.Artifact
	for _, b := range bundles {
		art, err := buildv1.NewBundleBuilder(b).BuildArtifact(context.Background())
		require.NoError(t, err)
		artifacts = append(artifacts, art)
	}
	gc := action.GenerateCatalog{
		BundleRepository: repo,
		Bundles:          artifacts,
		Log:              func(format string, v ...interface{}) {},
	}
	fsys, err := gc.Run(context.Background())
	require.NoError(t, err)
	return fsys
}

type provider struct {
	*oci.Store
}

func (p *provider) ReaderAt(ctx context.Context, desc ocispec.Descriptor) (content.ReaderAt, error) {
	rc, err := p.Fetch(ctx, desc)
	if err != nil {
		return nil, err
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, err
	}
	return &readerAt{bytes.NewReader(data)}, nil
}

type readerAt struct {
	*bytes.Reader
}

func (r *readerAt) Close() error { return nil }
