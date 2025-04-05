package catalog

import "github.com/joelanford/kpm/internal/kpm"

type Catalog interface {
	Annotations() map[string]string

	kpm.OCIMarshaler
	kpm.OCIUnmarshaler
}
