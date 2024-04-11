package tar

import (
	"archive/tar"
	"errors"
	"io"
	"io/fs"
	"time"
)

func Directory(w io.Writer, root fs.FS) error {
	tw := tar.NewWriter(w)
	defer tw.Close()
	return addFS(tw, root)
}

// addFS adds the files from fs.FS to the archive.
// It walks the directory tree starting at the root of the filesystem
// adding each file to the tar archive while maintaining the directory structure.
//
// NOTE: this function is copied from the Go standard library's tar package, and modified to add the
// header sanitization logic to avoid leaking local file system information and ensure reproducible output.
func addFS(tw *tar.Writer, fsys fs.FS) error {
	return fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		// TODO(#49580): Handle symlinks when fs.ReadLinkFS is available.
		if !info.Mode().IsRegular() {
			return errors.New("tar: cannot add non-regular file")
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = name

		/* MODIFICATION STARTS HERE */
		// Sanitize header fields to avoid leaking local file system information and ensure reproducible output.
		h.ChangeTime = time.Time{}
		h.AccessTime = time.Time{}
		h.ModTime = time.Time{}
		h.Uid = 0
		h.Gid = 0
		h.Uname = ""
		h.Gname = ""
		/* MODIFICATION STARTS HERE */

		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		f, err := fsys.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
}
