package i18n

import "golang.org/x/sys/windows"

var procUILang = windows.NewLazySystemDLL("kernel32.dll").NewProc("GetUserDefaultUILanguage")

// FromSystem is Russian when Windows' display language is Russian,
// Belarusian or Kazakh, English otherwise.
func FromSystem() Lang {
	id, _, _ := procUILang.Call()
	return fromPrimary(uint16(id) & 0x3ff)
}
