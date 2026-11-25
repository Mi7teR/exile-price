package pricer

import (
	"fmt"
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
// and lightning or fire and cold), junk mods and low tiers are left out
// unless nothing else is left, a weapon's local damage and speed are left
// to its DPS filters, the rest match their own stat. Every min is the roll
// less slack.
func rareFilters(lines []itemtext.ModLine, m mods, stats *trade.Stats, slack float64) []trade.StatFilter {
	var own, junkF []trade.StatFilter
	var elem, chaos, life float64
	at := func(id string, v float64, label string) trade.StatFilter {
		min := math.Floor(v * (1 - slack))
		return trade.StatFilter{ID: id, Value: &trade.MinMax{Min: &min}, Label: label}
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
		st, _, ok := m.resolve(stats, l.Text)
		if !ok || st.Value == 0 || m.weapon && inDPSRe.MatchString(l.Text) {
			continue
		}
		if isJunk(l.Text) || m.tiers[l.Text] >= lowTier {
			junkF = append(junkF, at(st.ID, st.Value, l.Text))
			continue
		}
		own = append(own, at(st.ID, st.Value, l.Text))
	}
	var out []trade.StatFilter
	if life > 0 {
		out = append(out, at(pseudoLife, life, fmt.Sprintf("%.0f life", life)))
	}
	if elem > 0 {
		out = append(out, at(pseudoElemRes, elem, fmt.Sprintf("%.0f%% res", elem)))
	}
	if chaos > 0 {
		out = append(out, at(pseudoChaosRes, chaos, fmt.Sprintf("%.0f%% chaos res", chaos)))
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

// lowTier and worse (tier numbers grow as rolls shrink) do not make an item
// worth more: a T5 mod is what any item of that base rolls.
const lowTier = 5

// mods is what rareFilters needs besides the lines: whether the item's
// local mods have their own trade stats, and the tiers an advanced copy
// shows.
type mods struct {
	local  bool           // weapon or armour: "(Local)" stats exist for its mods
	weapon bool           // local damage and attack speed are in the DPS filters
	tiers  map[string]int // mod text → tier; empty for a plain copy
}

// resolve finds the trade stat of a mod line, the local one first on
// weapons and armour: "+50 to Armour" on boots is the boots' own armour.
func (m mods) resolve(stats *trade.Stats, text string) (trade.Match, bool, bool) {
	if m.local {
		if st, ok := stats.Resolve("explicit", text+" (Local)"); ok {
			return st, true, true
		}
	}
	st, ok := stats.Resolve("explicit", text)
	return st, false, ok
}

// inDPSRe matches a weapon's own mods already counted in its physical and
// elemental DPS; "to Spells", "with Attack Skills" and chaos stay mods.
var inDPSRe = regexp.MustCompile(`^(Adds \d+ to \d+ (Physical|Fire|Cold|Lightning) Damage|\d+% increased (Physical Damage|Attack Speed))$`)

var (
	tierRe  = regexp.MustCompile(`\(Tier: (\d+)\)`)
	rangeRe = regexp.MustCompile(`\([\d.]+-[\d.]+\)`)
)

// tiers reads an advanced copy (Ctrl+Alt+C): each "{ ... (Tier: N) ... }"
// header applies to the mod lines under it, written as the parser writes
// them, without the "(min-max)" ranges.
func tiers(text string) map[string]int {
	out := map[string]int{}
	tier := 0
	for _, l := range strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "{"):
			tier = 0
			if m := tierRe.FindStringSubmatch(l); m != nil {
				tier, _ = strconv.Atoi(m[1])
			}
		case strings.HasPrefix(l, "--------") || l == "":
			tier = 0
		case tier > 0:
			out[rangeRe.ReplaceAllString(l, "")] = tier
		}
	}
	return out
}

var (
	physRe = regexp.MustCompile(`(?m)^Physical Damage: (.+)$`)
	elemRe = regexp.MustCompile(`(?m)^Elemental Damage: (.+)$`)
	apsRe  = regexp.MustCompile(`(?m)^Attacks per Second: ([\d.]+)`)
	hitRe  = regexp.MustCompile(`([\d.]+)-([\d.]+)`)
)

// weaponDPS reads the physical and elemental DPS a weapon shows.
func weaponDPS(text string) (pdps, edps float64) {
	m := apsRe.FindStringSubmatch(text)
	if m == nil {
		return 0, 0
	}
	aps, _ := strconv.ParseFloat(m[1], 64)
	avg := func(re *regexp.Regexp) float64 {
		m := re.FindStringSubmatch(text)
		if m == nil {
			return 0
		}
		var sum float64
		for _, h := range hitRe.FindAllStringSubmatch(m[1], -1) {
			lo, _ := strconv.ParseFloat(h[1], 64)
			hi, _ := strconv.ParseFloat(h[2], 64)
			sum += (lo + hi) / 2
		}
		return sum
	}
	return avg(physRe) * aps, avg(elemRe) * aps
}

// influence filters, from the parser's names to the trade site's pseudo stats.
var influenceStats = map[string]string{
	"shaper":      "pseudo.pseudo_has_shaper_influence",
	"elder":       "pseudo.pseudo_has_elder_influence",
	"crusader":    "pseudo.pseudo_has_crusader_influence",
	"eyrie":       "pseudo.pseudo_has_redeemer_influence",
	"basilisk":    "pseudo.pseudo_has_hunter_influence",
	"adjudicator": "pseudo.pseudo_has_warlord_influence",
}

// mustHave are filters a relaxed search keeps: the item's influences and
// its eldritch implicits, which a buyer pays for on their own.
func mustHave(it *itemtext.Item, stats *trade.Stats, slack float64) []trade.StatFilter {
	var out []trade.StatFilter
	for _, inf := range it.Influences {
		if id, ok := influenceStats[inf]; ok {
			out = append(out, trade.StatFilter{ID: id})
		}
	}
	for _, l := range it.ActiveImplicits() {
		if !l.Eater && !l.Exarch {
			continue
		}
		if st, ok := stats.Resolve("implicit", l.Text); ok && st.Value != 0 {
			min := math.Floor(st.Value * (1 - slack))
			out = append(out, trade.StatFilter{ID: st.ID, Value: &trade.MinMax{Min: &min}})
		}
	}
	return out
}
