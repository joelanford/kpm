package bundle

import (
	"github.com/blang/semver/v4"

	"github.com/joelanford/kpm/internal/kpm"
)

type Bundle interface {
	PackageName() string
	Version() semver.Version
	Annotations() map[string]string

	kpm.OCIMarshaler
	kpm.OCIUnmarshaler
}
