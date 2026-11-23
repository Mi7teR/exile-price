package pricer

import (
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/Mi7teR/poe-lib/itemtext"
	"github.com/Mi7teR/poe-pricecheck/internal/trade"
)

// Pseudo stats of the trade site, ids from the recorded /api/trade/data/stats.
const (
	pseudoElemRes  = "pseudo.pseudo_total_elemental_resistance"
	pseudoChaosRes = "pseudo.pseudo_total_chaos_resistance"
	pseudoLife     = "pseudo.pseudo_total_life"
)

var (
	// "+38% to Cold Resistance", "+12% to Fire and Lightning Resistances",
	// "+10% to all Elemental Resistances"; an advanced copy adds "(6-10)".
	resRe  = regexp.MustCompile(`^\+(\d+)(?:\([\d.-]+\))?% to (.+?) Resistances?$`)
	lifeRe = regexp.MustCompile(`^\+(\d+)(?:\([\d.-]+\))? to maximum Life$`)
)

// junk mods are worth next to nothing to a buyer; requiring them only
// narrows the search to the few items that happen to share them.
var junk = []string{
	"Light Radius",
	"Stun and Block Recovery",
	"Reflects ",
	"Attribute Requirements",
	"Stun Threshold",
	"per Enemy Killed",
}

func isJunk(text string) bool {
	for _, j := range junk {
		if strings.Contains(text, j) {
			return true
		}
	}
	return false
}

// rareFilters turns explicit mods into trade filters: resistances and flat
// life become their totals (a buyer does not care whether the 79% is cold
// and lightning or fire and cold), junk mods are left out unless nothing
// else is left, the rest match their own stat. Every min is the roll less
// slack.
func rareFilters(lines []itemtext.ModLine, stats *trade.Stats, slack float64) []trade.StatFilter {
	var own, junkF []trade.StatFilter
	var elem, chaos, life float64
	at := func(id string, v float64) trade.StatFilter {
		min := math.Floor(v * (1 - slack))
		return trade.StatFilter{ID: id, Value: &trade.MinMax{Min: &min}}
	}
	for _, l := range lines {
		if e, c, ok := resistances(l.Text); ok {
			elem, chaos = elem+e, chaos+c
			continue
		}
		if m := lifeRe.FindStringSubmatch(l.Text); m != nil {
			v, _ := strconv.ParseFloat(m[1], 64)
			life += v
			continue
		}
		m, ok := stats.Resolve("explicit", l.Text)
		if !ok || m.Value == 0 {
			continue
		}
		if isJunk(l.Text) {
			junkF = append(junkF, at(m.ID, m.Value))
			continue
		}
		own = append(own, at(m.ID, m.Value))
	}
	var out []trade.StatFilter
	if life > 0 {
		out = append(out, at(pseudoLife, life))
	}
	if elem > 0 {
		out = append(out, at(pseudoElemRes, elem))
	}
	if chaos > 0 {
		out = append(out, at(pseudoChaosRes, chaos))
	}
	out = append(out, own...)
	if len(out) == 0 {
		out = junkF
	}
	return out
}

// resistances reads a resistance mod as its elemental and chaos totals.
func resistances(text string) (elem, chaos float64, ok bool) {
	m := resRe.FindStringSubmatch(text)
	if m == nil {
		return 0, 0, false
	}
	v, _ := strconv.ParseFloat(m[1], 64)
	if m[2] == "all Elemental" {
		return 3 * v, 0, true
	}
	for _, part := range strings.Split(m[2], " and ") {
		switch part {
		case "Fire", "Cold", "Lightning":
			elem += v
		case "Chaos":
			chaos += v
		default:
			return 0, 0, false
		}
	}
	return elem, chaos, true
}
