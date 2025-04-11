package v1

import (
	"testing"
	"testing/fstest"

	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"github.com/stretchr/testify/require"
	"oras.land/oras-go/v2/content/memory"
)

func TestBundle_MarshalOCI(t *testing.T) {
	tests := []struct {
		name      string
		bundle    func(t *testing.T) Bundle
		expected  ocispec.Descriptor
		assertErr require.ErrorAssertionFunc
	}{
		{
			name: "succeeds",
			bundle: func(t *testing.T) Bundle {
				b, err := LoadFS(fstest.MapFS{
					"manifests/csv.yaml": &fstest.MapFile{Data: []byte(`
apiVersion: operators.coreos.com/v1alpha1
kind: ClusterServiceVersion
metadata:
  name: example.v1.2.3
spec:
  version: "1.2.3"
`)},
					"metadata/annotations.yaml": &fstest.MapFile{Data: []byte(`
annotations:
  operators.operatorframework.io.bundle.mediatype.v1: registry+v1
  operators.operatorframework.io.bundle.manifests.v1: manifests/
  operators.operatorframework.io.bundle.metadata.v1: metadata/
  operators.operatorframework.io.bundle.package.v1: example
`)},
				})
				require.NoError(t, err)
				return *b
			},
			expected: ocispec.Descriptor{
				MediaType: ocispec.MediaTypeImageManifest,
				// NOTE: DO NOT CHANGE THIS DIGEST if nothing in the bundle has changed.
				//   This ensures that bundles can always be rebuilt such that the digest
				//   never changes if input hasn't changed.
				//
				//   If you need to change something in the bundle, there should be a PR
				//   where the only changes are in the input/output of this test.
				Digest: digest.NewDigestFromEncoded(digest.SHA256, "02ac6235734e1f22385fa3b42c77076099ae4e5f930fb2e227186d49bdf1b916"),
				Size:   401,
			},
			assertErr: require.NoError,
		},
		{
			name:   "bundle zero-value",
			bundle: func(t *testing.T) Bundle { return Bundle{} },
			assertErr: func(t require.TestingT, err error, i ...interface{}) {
				require.ErrorContains(t, err, "cannot marshal uninitialized bundle")
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b := tt.bundle(t)
			target := memory.New()

			actual, err := b.MarshalOCI(t.Context(), target)

			tt.assertErr(t, err)
			require.Equal(t, tt.expected, actual)
		})
	}
}
