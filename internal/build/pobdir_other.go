//go:build !windows

package build

import (
	"os"
	"path/filepath"
)

func pobUserPath() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, "Documents", "Path of Building")
}
