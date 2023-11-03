package tar

import (
	"archive/tar"
	"fmt"
	"io"
	"io/fs"
	"time"
)

func Directory(w io.Writer, root fs.FS) error {
	tw := tar.NewWriter(w)
	defer tw.Close()

	buf := make([]byte, 1024)
	return fs.WalkDir(root, ".", func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		info, err := d.Info()
		if err != nil {
			return fmt.Errorf("get file info for %q: %w", path, err)
		}
		if !info.Mode().IsRegular() && !info.Mode().IsDir() {
			return fmt.Errorf("unsupported file type for %q: %s", path, info.Mode().Type())
		}

		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return fmt.Errorf("make tar header for file %q: %w", path, err)
		}
		hdr.Name = path
		hdr.AccessTime, hdr.ChangeTime, hdr.ModTime = time.Time{}, time.Time{}, time.Time{}
		hdr.Uid, hdr.Gid, hdr.Uname, hdr.Gname = 0, 0, "", ""
		if err := tw.WriteHeader(hdr); err != nil {
			return fmt.Errorf("write tar header for file %q: %w", path, err)
		}
		if info.Mode().IsRegular() {
			f, err := root.Open(path)
			if err != nil {
				return fmt.Errorf("open file %q: %w", path, err)
			}
			if _, err := io.CopyBuffer(tw, f, buf); err != nil {
				return fmt.Errorf("write file %q: %w", path, err)
			}
		}

		return nil
	})
}
