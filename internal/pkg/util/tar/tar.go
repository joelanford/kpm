package tar

import (
	"archive/tar"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

func Extract(r io.Reader, dest string) error {
	tr := tar.NewReader(r)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return err
		}

		// the target location where the dir/file should be created
		target := filepath.Join(dest, header.Name)

		// check the file type
		switch header.Typeflag {

		// if its a dir and it doesn't exist create it
		case tar.TypeDir:
			if _, err := os.Stat(target); err != nil {
				if err := os.MkdirAll(target, 0755); err != nil {
					return err
				}
			}

		// if it's a file create it
		case tar.TypeReg:
			f, err := os.OpenFile(target, os.O_CREATE|os.O_RDWR, os.FileMode(header.Mode))
			if err != nil {
				return err
			}

			// copy over contents
			if _, err := io.Copy(f, tr); err != nil {
				f.Close()
				return err
			}

			// manually close here after each file operation; defering would cause each file close
			// to wait until all operations have completed.
			f.Close()
		default:
			return errors.New("unsupported entry type: " + string(header.Typeflag))
		}
	}
	return nil
}

func Directory(w io.Writer, root fs.FS) error {
	tw := tar.NewWriter(w)
	defer tw.Close()
	return AddFS(tw, root)
}

// addFS adds the files from fs.FS to the archive.
// It walks the directory tree starting at the root of the filesystem
// adding each file to the tar archive while maintaining the directory structure.
//
// NOTE: this function is copied from the Go standard library's tar package, and modified to add the
// header sanitization logic to avoid leaking local file system information and ensure reproducible output.
func AddFS(tw *tar.Writer, fsys fs.FS) error {
	buf := make([]byte, 32*1024)
	return fs.WalkDir(fsys, ".", func(name string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		if name == "." {
			return nil
		}
		if !info.Mode().IsRegular() && !d.IsDir() {
			return errors.New("tar: cannot add non-regular file")
		}
		h, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		h.Name = name

		/* MODIFICATION STARTS HERE */
		// Sanitize header fields to avoid leaking local file system information and ensure reproducible output.
		//h.Format = tar.FormatPAX
		h.ChangeTime = time.Time{}
		h.AccessTime = time.Time{}
		h.ModTime = time.Time{}
		h.Uid = 0
		h.Gid = 0
		h.Uname = ""
		h.Gname = ""

		h.Mode = 0600
		if d.IsDir() {
			h.Mode = 0700
		}
		/* MODIFICATION STARTS HERE */

		if err := tw.WriteHeader(h); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		f, err := fsys.Open(name)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.CopyBuffer(tw, f, buf)
		return err
	})
}
