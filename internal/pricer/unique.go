package pricer

import (
	"context"
	"math"
	"strings"

	"github.com/Mi7teR/poe-lib/itemtext"
	"github.com/Mi7teR/poe-pricecheck/internal/trade"
)

// uniqueTradeMin: a unique poe.ninja puts under this (chaos) is not worth
// trade searches for its rolls.
const uniqueTradeMin = 20

// uniqueSlack: a unique is compared with ones rolled at least about as well.
const uniqueSlack = 0.05

// unique prices a unique on poe.ninja and, when it is worth it, on the
// trade site against copies rolled at least as well, with the same
// corruption: poe.ninja's price is the average roll.
func (p *Pricer) unique(ctx context.Context, it *itemtext.Item, base string) (*Result, error) {
	l, ok := p.Ninja().Unique(it.Name, base, links(it.Sockets))
	avg, err := p.fromNinja(it.Name, l, ok)
	ench := enchantFilters(it, p.Stats)
	if err != nil || l.Chaos < uniqueTradeMin && len(ench) == 0 || it.Unidentified || p.Trade == nil {
		return avg, err
	}
	cat := categories[it.ItemClass]
	m := mods{local: strings.HasPrefix(cat, "weapon") || strings.HasPrefix(cat, "armour.") && cat != "armour.quiver"}
	filters := uniqueFilters(it, m, p.Stats)
	if len(filters) == 0 && len(ench) == 0 {
		return avg, nil
	}
	q := trade.NewQuery()
	q.Query.Name = it.Name
	q.Query.Type = it.BaseName
	q.SetFilter("type_filters", "rarity", trade.Option{Option: "unique"})
	q.SetFilter("misc_filters", "corrupted", trade.Option{Option: boolOption(it.Corrupted)})
	if n := links(it.Sockets); n >= 5 {
		min := float64(n)
		q.SetFilter("socket_filters", "links", trade.MinMax{Min: &min})
	}
	if len(filters) > 0 { // first: the group a thin market relaxes
		q.Query.Stats = []trade.StatGroup{{Type: "and", Filters: filters}}
	}
	if len(ench) > 0 {
		q.Query.Stats = append(q.Query.Stats, trade.StatGroup{Type: "and", Filters: ench})
	}
	r, err := p.tradePrice(ctx, q, filters)
	if err != nil || len(r.Kept) == 0 && len(ench) == 0 { // the average beats nothing, but not a missing enchant
		if err != nil {
			avg.Note = strings.TrimSpace(avg.Note + " · trade: " + err.Error())
		}
		return avg, nil
	}
	r.Item, r.Avg = it.Name, l.Chaos
	for _, f := range ench {
		r.Searched = append(r.Searched, f.Label)
	}
	return r, nil
}

// uniqueFilters are the mods whose roll varies (all of them when the copy
// does not show ranges) and, on a corrupted unique, its implicits.
func uniqueFilters(it *itemtext.Item, m mods, stats *trade.Stats) []trade.StatFilter {
	lines := it.ActiveExplicits()
	ranged := false
	for _, l := range lines {
		ranged = ranged || l.HasRange
	}
	var out []trade.StatFilter
	add := func(kind string, l itemtext.ModLine) {
		if ranged && kind == "explicit" && !l.HasRange || strings.Contains(l.Text, "reduced") {
			return // fixed roll, or a downside: bigger is worse
		}
		var st trade.Match
		ok := false
		if kind == "explicit" {
			st, _, ok = m.resolve(stats, l.Text)
		} else {
			st, ok = stats.Resolve(kind, l.Text)
		}
		if !ok || st.Value <= 0 {
			return
		}
		min := math.Floor(st.Value * (1 - uniqueSlack))
		out = append(out, trade.StatFilter{ID: st.ID, Value: &trade.MinMax{Min: &min}})
	}
	for _, l := range lines {
		add("explicit", l)
	}
	if it.Corrupted {
		for _, l := range it.ActiveImplicits() {
			add("implicit", l)
		}
	}
	return out
}

func boolOption(b bool) string {
	if b {
		return "true"
	}
	return "false"
}
