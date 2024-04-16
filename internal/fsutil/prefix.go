package fsutil

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/util/sets"
)

type prefixFS struct {
	fsys       fs.FS
	prefix     string
	prefixDirs sets.Set[string]
}

func Prefix(fsys fs.FS, prefix string) (fs.FS, error) {
	if prefix != filepath.Clean(prefix) || len(prefix) == 0 {
		return nil, fmt.Errorf("invalid prefix %q", prefix)
	}
	prefix = strings.TrimPrefix(prefix, string(filepath.Separator))
	prefix = strings.TrimSuffix(prefix, string(filepath.Separator))
	prefixSegments := strings.Split(prefix, string(filepath.Separator))
	prefixDirs := sets.New[string]()
	for i := range prefixSegments {
		dirPath := strings.Join(prefixSegments[:i+1], string(filepath.Separator))
		prefixDirs.Insert(dirPath)
	}
	return &prefixFS{fsys, prefix, prefixDirs}, nil
}

func (p *prefixFS) Open(name string) (fs.File, error) {
	name = filepath.Clean(name)
	if strings.HasPrefix(name, p.prefix) {
		delegatedName := strings.TrimPrefix(name, p.prefix)
		if delegatedName == "" {
			delegatedName = "."
		}
		if strings.HasPrefix(delegatedName, string(filepath.Separator)) {
			delegatedName = strings.TrimPrefix(delegatedName, string(filepath.Separator))
		}
		return p.fsys.Open(delegatedName)
	}
	if p.prefixDirs.Has(name) {
		return &prefixDir{name}, nil
	}
	if name == "." {
		return &prefixDir{name}, nil
	}
	return nil, os.ErrNotExist
}

func (p *prefixFS) ReadDir(name string) ([]fs.DirEntry, error) {
	var children []fs.DirEntry
	if strings.HasPrefix(name, p.prefix) {
		delegatedName := strings.TrimPrefix(name, p.prefix)
		if delegatedName == "" {
			delegatedName = "."
		}
		if strings.HasPrefix(delegatedName, string(filepath.Separator)) {
			delegatedName = strings.TrimPrefix(delegatedName, string(filepath.Separator))
		}
		return fs.ReadDir(p.fsys, delegatedName)
	}
	if name == "." {
		for _, d := range sets.List(p.prefixDirs) {
			dSegmentCount := strings.Count(d, string(filepath.Separator))
			if dSegmentCount == 0 {
				children = append(children, &prefixDirEntry{d})
			}
		}
		return children, nil
	}
	if p.prefixDirs.Has(name) {
		nameSegmentCount := strings.Count(name, string(filepath.Separator))
		for _, d := range sets.List(p.prefixDirs) {
			dSegmentCount := strings.Count(d, string(filepath.Separator))
			if nameSegmentCount+1 == dSegmentCount {
				children = append(children, &prefixDirEntry{strings.TrimSuffix(d, string(filepath.Separator))})
			}
		}
		return children, nil
	}
	return nil, os.ErrNotExist
}

type prefixDir struct {
	name string
}

func (p prefixDir) Stat() (fs.FileInfo, error) {
	return &prefixDirInfo{p.name}, nil
}

func (p prefixDir) Read(bytes []byte) (int, error) {
	//TODO implement me
	panic("implement read")
}

func (p prefixDir) Close() error {
	return nil
}

type prefixDirInfo struct {
	name string
}

func (p prefixDirInfo) Name() string {
	return filepath.Base(p.name)
}

func (p prefixDirInfo) Size() int64 {
	return 0
}

func (p prefixDirInfo) Mode() fs.FileMode {
	return fs.ModeDir | 0755
}

func (p prefixDirInfo) ModTime() time.Time {
	return time.Time{}
}

func (p prefixDirInfo) IsDir() bool {
	return true
}

func (p prefixDirInfo) Sys() any {
	return nil
}

type prefixDirEntry struct {
	name string
}

func (p prefixDirEntry) Name() string {
	return filepath.Base(p.name)
}

func (p prefixDirEntry) IsDir() bool {
	return true
}

func (p prefixDirEntry) Type() fs.FileMode {
	return os.ModeDir
}

func (p prefixDirEntry) Info() (fs.FileInfo, error) {
	return &prefixDirInfo{p.name}, nil
}
