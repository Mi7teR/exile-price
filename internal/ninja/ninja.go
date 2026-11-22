// Package ninja loads poe.ninja's dense overviews into an in-memory price index.
package ninja

import (
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

// Line is one priced entry. Variant is poe.ninja's free-form qualifier:
// map tier, gem level/quality, unique links or base.
type Line struct {
	Type    string  // overview type: Currency, UniqueArmour, SkillGem, ...
	Name    string  `json:"name"`
	Variant string  `json:"variant"`
	Chaos   float64 `json:"chaos"`
}

type overview struct {
	Type  string `json:"type"`
	Lines []Line `json:"lines"`
}

type dense struct {
	CurrencyOverviews []overview `json:"currencyOverviews"`
	ItemOverviews     []overview `json:"itemOverviews"`
}

// Index maps an item name to every priced variant of it.
type Index struct {
	byName map[string][]Line
}

// Parse builds an index from a dense overviews response body.
func Parse(r io.Reader) (*Index, error) {
	var d dense
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return nil, fmt.Errorf("ninja: decode: %w", err)
	}
	idx := &Index{byName: map[string][]Line{}}
	for _, group := range [][]overview{d.CurrencyOverviews, d.ItemOverviews} {
		for _, o := range group {
			for _, l := range o.Lines {
				l.Type = o.Type
				idx.byName[l.Name] = append(idx.byName[l.Name], l)
			}
		}
	}
	return idx, nil
}

// Lookup returns all variants priced under name.
func (i *Index) Lookup(name string) []Line { return i.byName[name] }

// Best picks the variant whose qualifier mentions every hint (case-insensitive);
// without a match it falls back to the cheapest line, the common non-special
// variant of a unique.
func (i *Index) Best(name string, hints ...string) (Line, bool) {
	lines := i.byName[name]
	if len(lines) == 0 {
		return Line{}, false
	}
	for _, l := range lines {
		if matchesAll(l.Variant, hints) {
			return l, true
		}
	}
	best := lines[0]
	for _, l := range lines[1:] {
		if l.Chaos < best.Chaos {
			best = l
		}
	}
	return best, true
}

func matchesAll(variant string, hints []string) bool {
	if len(hints) == 0 {
		return variant == ""
	}
	v := strings.ToLower(variant)
	for _, h := range hints {
		if !strings.Contains(v, strings.ToLower(h)) {
			return false
		}
	}
	return true
}

// Chaos returns the chaos value of a currency by its full name.
func (i *Index) Chaos(currencyName string) (float64, bool) {
	if currencyName == "Chaos Orb" {
		return 1, true
	}
	for _, l := range i.byName[currencyName] {
		if l.Type == "Currency" {
			return l.Chaos, true
		}
	}
	return 0, false
}
