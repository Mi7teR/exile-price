package pricer

import (
	"context"
	"strings"

	"github.com/Mi7teR/exile-calc/itemtext"
	"github.com/Mi7teR/exile-price/internal/trade"
)

// passivesStat is the cluster enchant "Adds # Passive Skills".
const passivesStat = "enchant.stat_3086156145"

func isCluster(it *itemtext.Item) bool { return strings.HasSuffix(it.BaseName, " Cluster Jewel") }

// cluster prices a rare cluster jewel the way buyers search for one: the
// same size and small passive (its enchants) and the same notables. The
// "Added Small Passive Skills also grant" mods are filler.
func (p *Pricer) cluster(ctx context.Context, it *itemtext.Item) (*Result, error) {
	base, notables := clusterFilters(it, p.Stats)
	q := trade.NewQuery()
	q.SetFilter("type_filters", "category", trade.Option{Option: "jewel.cluster"})
	q.SetFilter("type_filters", "rarity", trade.Option{Option: "nonunique"})
	q.Query.Type = it.BaseName
	q.Query.Stats = []trade.StatGroup{{Type: "and", Filters: notables}, {Type: "and", Filters: base}}
	r, err := p.tradePrice(ctx, q, notables)
	if r != nil {
		r.Item = it.Name + ", " + it.BaseName
	}
	return r, err
}

// clusterFilters: base is the size and small passive (the enchants),
// notables the "1 Added Passive Skill is ..." mods.
func clusterFilters(it *itemtext.Item, stats *trade.Stats) (base, notables []trade.StatFilter) {
	for _, l := range it.Enchants {
		text := l.Text
		if text == "1 Added Passive Skill is a Jewel Socket" { // the trade site has only the plural
			text = "1 Added Passive Skills are Jewel Sockets"
		}
		st, ok := stats.Resolve("enchant", text)
		if !ok {
			continue
		}
		if st.ID == passivesStat {
			n := st.Value
			base = append(base, trade.StatFilter{ID: st.ID, Value: &trade.MinMax{Min: &n, Max: &n}})
			continue
		}
		base = append(base, trade.Exact(st.ID))
	}
	for _, l := range it.ActiveExplicits() {
		if !strings.HasPrefix(l.Text, "1 Added Passive Skill is ") {
			continue
		}
		if st, ok := stats.Resolve("explicit", l.Text); ok {
			notables = append(notables, trade.Exact(st.ID))
		}
	}
	return base, notables
}
