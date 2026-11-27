// Package i18n picks English or Russian for what the user sees. English
// strings are the keys; a missing Russian entry falls back to English.
package i18n

import (
	"fmt"
	"strings"
	"sync/atomic"
)

type Lang int

const (
	EN Lang = iota
	RU
)

var lang atomic.Int32 // a Lang; the zero value is EN

// Set chooses the language; the settings may change it while the app runs.
func Set(l Lang) { lang.Store(int32(l)) }

func Current() Lang { return Lang(lang.Load()) }

// T translates an English string.
func T(en string) string {
	if Current() == RU {
		if s, ok := ru[en]; ok {
			return s
		}
	}
	return en
}

// Tf translates an English format string and fills it in.
func Tf(en string, args ...any) string { return fmt.Sprintf(T(en), args...) }

// Count writes "n noun" with the noun in the right plural form, e.g.
// Count(3, "listing") is "3 listings" or "3 лота".
func Count(n int, noun string) string {
	if Current() == RU {
		if f, ok := ruNouns[noun]; ok {
			return fmt.Sprintf("%d %s", n, ruPlural(n, f))
		}
	}
	if n == 1 {
		return fmt.Sprintf("%d %s", n, noun)
	}
	return fmt.Sprintf("%d %ss", n, noun)
}

// ruPlural picks the form for 1, 2–4 and 5+ (11–14 take the last).
func ruPlural(n int, f [3]string) string {
	n %= 100
	switch {
	case n%10 == 1 && n != 11:
		return f[0]
	case n%10 >= 2 && n%10 <= 4 && (n < 12 || n > 14):
		return f[1]
	}
	return f[2]
}

// Parse reads the language setting: "ru" is Russian, anything else English.
func Parse(code string) Lang {
	if strings.EqualFold(code, "ru") {
		return RU
	}
	return EN
}
