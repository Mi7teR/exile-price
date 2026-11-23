// Package fixers removes price-fixer listings from a trade result page.
// Pure functions: prices arrive already converted to chaos.
package fixers

import (
	"sort"
	"time"
)

type Lot struct {
	Account   string
	Signature string // same item listed again by the same account → same signature
	Chaos     float64
	Indexed   time.Time
}

type Dropped struct {
	Lot    Lot
	Reason string
}

type Config struct {
	CheapRatio   float64       // a lot under CheapRatio × reference is suspect
	StaleAge     time.Duration // ... and dropped when listed longer than this
	MinReference float64       // below this reference (chaos) the cheap rule is off: cheap items are real
	OldAge       time.Duration // lots listed longer are left out ...
	MinFresh     int           // ... when at least this many fresher ones remain
}

var Default = Config{CheapRatio: 0.5, StaleAge: 24 * time.Hour, MinReference: 10, OldAge: 7 * 24 * time.Hour, MinFresh: 5}

type Result struct {
	Kept      []Lot
	Dropped   []Dropped
	Reference float64 // median before dropping
	Low, Mid  float64 // p25, p50 of kept lots
}

const (
	ReasonDuplicate = "повтор того же лота"
	ReasonCheapOld  = "дешёвый и давно висит"
	ReasonOld       = "висит больше недели"
)

// Clean dedupes by account+signature, then drops lots that are both cheap
// against the median and old: a real cheap item would have been bought.
// In a cheap market (median under MinReference) nothing is dropped as a
// fixer, nor when a quarter of the lots or more look like one.
// Lots older than OldAge go too while MinFresh fresher ones remain.
func Clean(lots []Lot, cfg Config, now time.Time) Result {
	var r Result
	seen := map[string]bool{}
	var uniq []Lot
	for _, l := range lots {
		k := l.Account + "\x00" + l.Signature
		if seen[k] {
			r.Dropped = append(r.Dropped, Dropped{l, ReasonDuplicate})
			continue
		}
		seen[k] = true
		uniq = append(uniq, l)
	}
	r.Reference = quantile(prices(uniq), 0.5)
	suspect := func(l Lot) bool {
		return r.Reference >= cfg.MinReference && l.Chaos < r.Reference*cfg.CheapRatio && now.Sub(l.Indexed) > cfg.StaleAge
	}
	n := 0
	for _, l := range uniq {
		if suspect(l) {
			n++
		}
	}
	// fixers are a few baits under a market; a quarter of the page or more
	// that cheap is the market itself (spike, 2026-11-23: eight 3–10c abyss
	// jewels were dropped under a 20c median)
	market := n*4 >= len(uniq)
	for _, l := range uniq {
		if !market && suspect(l) {
			r.Dropped = append(r.Dropped, Dropped{l, ReasonCheapOld})
			continue
		}
		r.Kept = append(r.Kept, l)
	}
	// an asking price nobody took for a week says little about today's
	// price; the fresh lots are already on this page, no search more
	fresh := 0
	for _, l := range r.Kept {
		if now.Sub(l.Indexed) <= cfg.OldAge {
			fresh++
		}
	}
	if cfg.MinFresh > 0 && fresh >= cfg.MinFresh && fresh < len(r.Kept) {
		kept := r.Kept[:0:0]
		for _, l := range r.Kept {
			if now.Sub(l.Indexed) > cfg.OldAge {
				r.Dropped = append(r.Dropped, Dropped{l, ReasonOld})
				continue
			}
			kept = append(kept, l)
		}
		r.Kept = kept
	}
	p := prices(r.Kept)
	r.Low, r.Mid = quantile(p, 0.25), quantile(p, 0.5)
	return r
}

func prices(lots []Lot) []float64 {
	p := make([]float64, len(lots))
	for i, l := range lots {
		p[i] = l.Chaos
	}
	sort.Float64s(p)
	return p
}

// quantile of sorted values, linear interpolation; 0 for none.
func quantile(sorted []float64, q float64) float64 {
	if len(sorted) == 0 {
		return 0
	}
	pos := q * float64(len(sorted)-1)
	i := int(pos)
	if i+1 >= len(sorted) {
		return sorted[i]
	}
	return sorted[i] + (sorted[i+1]-sorted[i])*(pos-float64(i))
}
