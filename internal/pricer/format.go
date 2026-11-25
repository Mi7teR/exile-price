package pricer

import (
	"fmt"
	"strings"

	"github.com/Mi7teR/poe-pricecheck/internal/i18n"

	"github.com/Mi7teR/poe-pricecheck/internal/fixers"
)

// Line is the one-line answer the overlay shows, e.g.
// "~6–9 div · 14 лотов, 3 фиксера" or "~12c · poe.ninja".
func (r *Result) Line(divine float64) string {
	if r.Source == "poe.ninja" {
		if r.Base > 0 {
			return i18n.Tf("base ~%s · poe.ninja", Money(r.Base, divine))
		}
		return "~" + Money(r.Mid, divine) + " · poe.ninja"
	}
	base := ""
	if r.Base > 0 {
		base = i18n.Tf(" · base ~%s", Money(r.Base, divine))
	}
	if r.Avg > 0 {
		base += i18n.Tf(" · poe.ninja ~%s", Money(r.Avg, divine))
	}
	if len(r.Kept) == 0 {
		return i18n.Tf("no listings · %d on the site", r.Total) + base
	}
	n := len(r.Kept)
	s := "~" + MoneyRange(r.Low, r.Mid, divine) + " · " + i18n.Count(n, "listing")
	if n := r.Fixers(); n > 0 {
		s += ", " + i18n.Count(n, "fixer")
	}
	if r.Relaxed || len(r.Kept) < 5 {
		s += i18n.T(" · rough estimate")
	}
	return s + base + r.searched()
}

// Fixers counts lots dropped as price fixers (duplicates not included).
func (r *Result) Fixers() int {
	n := 0
	for _, d := range r.Dropped {
		if d.Reason == fixers.ReasonCheapOld {
			n++
		}
	}
	return n
}

// Money writes chaos as divines from one divine up, else as chaos.
func Money(chaos, divine float64) string {
	if divine > 0 && chaos >= divine {
		return fmt.Sprintf("%.1f div", chaos/divine)
	}
	if chaos < 10 {
		return fmt.Sprintf("%.1fc", chaos)
	}
	return fmt.Sprintf("%.0fc", chaos)
}

func MoneyRange(lo, hi, divine float64) string {
	a, b := Money(lo, divine), Money(hi, divine)
	if a == b {
		return a
	}
	return a + "–" + b
}

// searchedShown is how many searched mods the line names; the rest is "+N".
const searchedShown = 3

// searched names what a rare search asked for, so a player sees why the
// price is what it is: " · searched: 68 life, 79% res, … +2".
func (r *Result) searched() string {
	if len(r.Searched) == 0 {
		return ""
	}
	s := strings.Join(r.Searched[:min(len(r.Searched), searchedShown)], ", ")
	if n := len(r.Searched) - searchedShown; n > 0 {
		s += fmt.Sprintf(" +%d", n)
	}
	return i18n.Tf(" · searched: %s", s)
}
