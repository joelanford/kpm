package fsutil

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

func Write(rootPath string, fsys fs.FS) error {
	buf := make([]byte, 32*1024)
	return fs.WalkDir(fsys, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == "." {
			return nil
		}

		if !d.IsDir() && !d.Type().IsRegular() {
			return fmt.Errorf("unexpected file type: %s", d.Type())
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("error getting info for entry %q: %w", path, err)
		}

		write := func() error {
			if d.IsDir() {
				return os.MkdirAll(filepath.Join(rootPath, path), 0755)
			}
			in, err := fsys.Open(path)
			if err != nil {
				return err
			}
			out, err := os.Create(filepath.Join(rootPath, path))
			defer out.Close()
			if err != nil {
				return err
			}
			if _, err := io.CopyBuffer(out, in, buf); err != nil {
				return err
			}
			return nil
		}
		if err := write(); err != nil {
			return fmt.Errorf("error writing entry %q: %w", path, err)
		}
		if err := os.Chtimes(filepath.Join(rootPath, path), time.Time{}, time.Time{}); err != nil {
			return fmt.Errorf("error fixing up times for entry %q: %w", path, err)
		}
		if err := os.Chmod(filepath.Join(rootPath, path), info.Mode()); err != nil {
			return fmt.Errorf("error fixing up permissions for entry %q: %w", path, err)
		}
		return nil
	})
}
