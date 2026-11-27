package build

import (
	"path/filepath"

	"golang.org/x/sys/windows"
)

func pobUserPath() string {
	docs, err := windows.KnownFolderPath(windows.FOLDERID_Documents, 0)
	if err != nil {
		return ""
	}
	return filepath.Join(docs, "Path of Building")
}
