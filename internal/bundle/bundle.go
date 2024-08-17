package bundle

import (
	"io/fs"

	"github.com/blang/semver/v4"
)

type Bundle interface {
	FS() fs.FS
	PackageName() string
	Version() semver.Version
	Annotations() map[string]string
}
