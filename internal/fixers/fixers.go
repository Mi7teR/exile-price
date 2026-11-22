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
	CheapRatio float64       // a lot under CheapRatio × reference is suspect
	StaleAge   time.Duration // ... and dropped when listed longer than this
}

var Default = Config{CheapRatio: 0.5, StaleAge: 24 * time.Hour}

type Result struct {
	Kept      []Lot
	Dropped   []Dropped
	Reference float64 // median before dropping
	Low, Mid  float64 // p25, p50 of kept lots
}

const (
	ReasonDuplicate = "повтор того же лота"
	ReasonCheapOld  = "дешёвый и давно висит"
)

// Clean dedupes by account+signature, then drops lots that are both cheap
// against the median and old: a real cheap item would have been bought.
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
	for _, l := range uniq {
		if l.Chaos < r.Reference*cfg.CheapRatio && now.Sub(l.Indexed) > cfg.StaleAge {
			r.Dropped = append(r.Dropped, Dropped{l, ReasonCheapOld})
			continue
		}
		r.Kept = append(r.Kept, l)
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
