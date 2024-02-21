package tar

import (
	"archive/tar"
	"io"
	"io/fs"
)

func Directory(w io.Writer, root fs.FS) error {
	tw := tar.NewWriter(w)
	defer tw.Close()
	return tw.AddFS(root)
}
