package bundle

import (
	"io"
	"io/fs"

	"github.com/blang/semver/v4"
	"github.com/containers/image/v5/docker/reference"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type Bundle interface {
	FS() fs.FS
	PackageName() string
	Version() semver.Version
	Annotations() map[string]string
	WriteOCIArchive(w io.Writer, tagged reference.NamedTagged) (ocispec.Descriptor, error)
}
