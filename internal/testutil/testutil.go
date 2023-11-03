package testutil

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestdataFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := testdata.ReadFile(filepath.Join("testdata", path))
	require.NoError(t, err)
	return data
}
