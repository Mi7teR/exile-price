// Package ninja loads poe.ninja's dense overviews into an in-memory price index.
package ninja

import (
	"encoding/json"
	"fmt"
	"io"
	"strconv"
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

// Best returns the line for name: the one with no variant, else the cheapest
// (the common, non-special variant).
func (i *Index) Best(name string) (Line, bool) {
	lines := i.byName[name]
	if len(lines) == 0 {
		return Line{}, false
	}
	best := lines[0]
	for _, l := range lines {
		if l.Variant == "" {
			return l, true
		}
		if l.Chaos < best.Chaos {
			best = l
		}
	}
	return best, true
}

// Variant returns the line whose variant is exactly v.
func (i *Index) Variant(name, v string) (Line, bool) {
	for _, l := range i.byName[name] {
		if l.Variant == v {
			return l, true
		}
	}
	return Line{}, false
}

// Gem finds a gem by level, quality and corruption. poe.ninja writes
// "20/20c", "20" (no quality), "4c"; without an exact line it falls back to
// the same level and corruption with the highest quality not above ours.
func (i *Index) Gem(name string, level, quality int, corrupted bool) (Line, bool) {
	v := strconv.Itoa(level)
	if quality > 0 {
		v += "/" + strconv.Itoa(quality)
	}
	if corrupted {
		v += "c"
	}
	if l, ok := i.Variant(name, v); ok {
		return l, true
	}
	var best Line
	bestQ := -1
	for _, l := range i.byName[name] {
		lv, q, c := gemVariant(l.Variant)
		if lv == level && c == corrupted && q <= quality && q > bestQ {
			best, bestQ = l, q
		}
	}
	return best, bestQ >= 0
}

func gemVariant(v string) (level, quality int, corrupted bool) {
	corrupted = strings.HasSuffix(v, "c")
	v = strings.TrimSuffix(v, "c")
	if i := strings.IndexByte(v, '/'); i >= 0 {
		quality, _ = strconv.Atoi(v[i+1:])
		v = v[:i]
	}
	level, _ = strconv.Atoi(v)
	return level, quality, corrupted
}

// Unique finds a unique by base and links: "Occultist's Vestment, 6L" when
// links >= 5, else "Occultist's Vestment"; then any line on that base; then Best.
func (i *Index) Unique(name, base string, links int) (Line, bool) {
	if links >= 5 {
		if l, ok := i.Variant(name, fmt.Sprintf("%s, %dL", base, links)); ok {
			return l, true
		}
	}
	if l, ok := i.Variant(name, base); ok {
		return l, true
	}
	return i.Best(name)
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

// influenceNames maps item text influences to poe.ninja's base variant names.
// The Searing Exarch and Eater of Worlds have no base prices there.
var influenceNames = map[string]string{
	"shaper": "Shaper", "elder": "Elder", "crusader": "Crusader",
	"eyrie": "Redeemer", "basilisk": "Hunter", "adjudicator": "Warlord",
}

// Base prices a crafting base by item level (82–85, "86+") and influences,
// e.g. "Paladin Boots" with variant "86+, Elder/Crusader". Bases below item
// level 82 are not priced on poe.ninja.
func (i *Index) Base(name string, ilvl int, influences []string) (Line, bool) {
	if ilvl < 82 {
		return Line{}, false
	}
	v := strconv.Itoa(ilvl)
	if ilvl >= 86 {
		v = "86+"
	}
	var inf []string
	for _, s := range influences {
		n, ok := influenceNames[s]
		if !ok {
			return Line{}, false
		}
		inf = append(inf, n)
	}
	want := []string{v}
	switch len(inf) {
	case 0:
	case 1:
		want = []string{v + ", " + inf[0]}
	case 2: // the order on poe.ninja is not fixed
		want = []string{v + ", " + inf[0] + "/" + inf[1], v + ", " + inf[1] + "/" + inf[0]}
	default:
		return Line{}, false
	}
	for _, l := range i.byName[name] {
		if l.Type != "BaseType" {
			continue
		}
		for _, w := range want {
			if l.Variant == w {
				return l, true
			}
		}
	}
	return Line{}, false
}
