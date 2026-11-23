//go:build !windows

package i18n

import (
	"os"
	"strings"
)

// FromSystem reads LC_ALL, LC_MESSAGES or LANG ("ru_RU.UTF-8").
func FromSystem() Lang {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			for code, id := range map[string]uint16{"ru": 0x19, "be": 0x23, "kk": 0x3f} {
				if strings.HasPrefix(v, code) {
					return fromPrimary(id)
				}
			}
			return EN
		}
	}
	return EN
}
