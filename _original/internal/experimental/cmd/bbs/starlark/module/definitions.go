package module

import (
	"path/filepath"

	"github.com/adrg/xdg"
)

var (
	globalConfigDirPath string
	userConfigDirPath   = xdg.ConfigHome
)

func SystemPath(version, name string) string {
	return filePath(globalConfigDirPath, version, name)
}

func UserPath(version, name string) string {
	return filePath(userConfigDirPath, version, name)
}

func filePath(base, version, name string) string {
	return filepath.Join(base, "kpm", "module", version, name)
}
