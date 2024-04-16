package v1

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"path/filepath"
	"strings"
	"testing/fstest"

	"github.com/joelanford/kpm/internal/tar"
	"github.com/joelanford/kpm/oci"
	"github.com/opencontainers/go-digest"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type ArtifactBuilder interface {
	BuildArtifact(ctx context.Context) (oci.Artifact, error)
}

type BuildOption func(*buildOptions)

func WithSpecReader(r io.Reader) BuildOption {
	return func(opts *buildOptions) {
		opts.SpecReader = r
	}
}

func WithLog(log func(string, ...interface{})) BuildOption {
	return func(opts *buildOptions) {
		opts.Log = log
	}
}

type buildOptions struct {
	SpecReader io.Reader
	Log        func(string, ...interface{})
}

func getConfigData(annotations map[string]string, blobData []byte) ([]byte, error) {
	config := ocispec.Image{
		Config: ocispec.ImageConfig{
			Labels: annotations,
		},
		RootFS: ocispec.RootFS{
			Type:    "layers",
			DiffIDs: []digest.Digest{digest.FromBytes(blobData)},
		},
		History: []ocispec.History{{
			CreatedBy: "kpm",
		}},
		Platform: ocispec.Platform{
			OS: "linux",
		},
	}
	return json.Marshal(config)
}

func getBlobData(fsys fs.FS) ([]byte, error) {
	buf := &bytes.Buffer{}
	gzw := gzip.NewWriter(buf)

	if err := tar.Directory(gzw, fsys); err != nil {
		return nil, err
	}
	if err := gzw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

type multiFS struct {
	fsMap map[string]fs.FS
}

func newMultiFS() *multiFS {
	return &multiFS{
		fsMap: make(map[string]fs.FS),
	}
}

func (fsys *multiFS) mount(path string, pathFs fs.FS) error {
	before, after, _ := strings.Cut(path, string(filepath.Separator))
	if after == "" {
		if _, ok := fsys.fsMap[before]; ok {
			return fmt.Errorf("mount point %q already exists", before)
		}
		fsys.fsMap[before] = pathFs
		return nil
	}

	beforeFs, ok := fsys.fsMap[before]
	if !ok {
		beforeMultiFS := newMultiFS()
		_ = beforeMultiFS.mount(after, pathFs)
		fsys.fsMap[before] = beforeMultiFS
		return nil
	}

	beforeMultiFS, ok := beforeFs.(*multiFS)
	if !ok {
		return fmt.Errorf("mount point %q already exists", before)
	}
	beforeMultiFS.mount(after, pathFs)
	return nil
}

func (r *multiFS) Open(name string) (fs.File, error) {
	name = filepath.Clean(name)
	if name == "." {
		mapFS := fstest.MapFS{}
		for path := range r.fsMap {
			mapFS[path] = &fstest.MapFile{Mode: fs.ModeDir}
		}
		return mapFS.Open(name)
	}
	if fsys, ok := r.fsMap[name]; ok {
		return fsys.Open(".")
	}

	for path, fsys := range r.fsMap {
		prefix := path + string(filepath.Separator)
		if strings.HasPrefix(name, prefix) {
			return fsys.Open(strings.TrimPrefix(name, prefix))
		}
	}

	return nil, fs.ErrNotExist
}
