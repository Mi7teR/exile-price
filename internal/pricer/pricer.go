// Package pricer routes an item to poe.ninja or the trade site and builds
// the one-line answer.
package pricer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/Mi7teR/poe-lib/itemtext"
	"github.com/Mi7teR/poe-pricecheck/internal/fixers"
	"github.com/Mi7teR/poe-pricecheck/internal/ninja"
	"github.com/Mi7teR/poe-pricecheck/internal/trade"
)

type Pricer struct {
	League     string
	Ninja      *ninja.Index
	Trade      *trade.Client
	Stats      *trade.Stats
	Currencies map[string]string // trade currency id → full name, from /data/static
	Parser     *itemtext.Parser
	Fixers     fixers.Config
	RollSlack  float64 // filter min = roll × (1 − RollSlack)
	Now        func() time.Time
}

type Result struct {
	Source   string // "poe.ninja" or "trade"
	Chaos    float64
	Low, Mid float64 // trade: p25, p50 of kept lots, chaos
	Total    int     // trade: matches on the site
	Kept     []fixers.Lot
	Dropped  []fixers.Dropped
	Link     string
	Note     string
	Timings  []Timing
}

type Timing struct {
	Step string
	Took time.Duration
}

var ErrNotItem = errors.New("pricer: text is not a PoE item")

// Price prices one item text (Ctrl+C or Ctrl+Alt+C).
func (p *Pricer) Price(ctx context.Context, text string) (*Result, error) {
	h, err := readHeader(text)
	if err != nil {
		return nil, err
	}
	switch h.rarity {
	case "Currency", "Divination Card":
		return p.fromNinja(h.name)
	case "Gem":
		return p.fromNinja(h.name, gemVariant(text))
	}
	t0 := time.Now()
	it, err := p.Parser.Parse(text)
	if err != nil {
		return nil, err
	}
	parsed := time.Since(t0)
	var r *Result
	switch it.Rarity {
	case itemtext.Unique:
		r, err = p.fromNinja(it.Name)
	case itemtext.Rare:
		r, err = p.rare(ctx, it)
	default:
		return nil, fmt.Errorf("pricer: %s %s is not priced in the spike", strings.ToLower(string(it.Rarity)), it.ItemClass)
	}
	if r != nil {
		r.Timings = append([]Timing{{"parse", parsed}}, r.Timings...)
	}
	return r, err
}

func (p *Pricer) fromNinja(name string, hints ...string) (*Result, error) {
	l, ok := p.Ninja.Best(name, hints...)
	if !ok {
		return nil, fmt.Errorf("pricer: %q not on poe.ninja", name)
	}
	r := &Result{Source: "poe.ninja", Chaos: l.Chaos, Mid: l.Chaos, Low: l.Chaos}
	if l.Variant != "" {
		r.Note = "вариант: " + l.Variant
	}
	return r, nil
}

var categories = map[string]string{
	"Boots": "armour.boots", "Gloves": "armour.gloves", "Helmets": "armour.helmet",
	"Body Armours": "armour.chest", "Shields": "armour.shield", "Quivers": "armour.quiver",
	"Rings": "accessory.ring", "Amulets": "accessory.amulet", "Belts": "accessory.belt",
	"Jewels": "jewel",
}

const fetchLots = 2 * trade.FetchLimit

func (p *Pricer) rare(ctx context.Context, it *itemtext.Item) (*Result, error) {
	cat, ok := categories[it.ItemClass]
	if !ok {
		return nil, fmt.Errorf("pricer: rare %s is not priced in the spike", it.ItemClass)
	}
	var filters []trade.StatFilter
	add := func(kind string, lines []itemtext.ModLine) {
		for _, l := range lines {
			m, ok := p.Stats.Resolve(kind, l.Text)
			if !ok || m.Value == 0 {
				continue
			}
			min := math.Floor(m.Value * (1 - p.RollSlack))
			filters = append(filters, trade.StatFilter{ID: m.ID, Value: &trade.MinMax{Min: &min}})
		}
	}
	add("implicit", it.ActiveImplicits())
	add("explicit", it.ActiveExplicits())
	if len(filters) == 0 {
		return nil, errors.New("pricer: no mod matched a trade stat")
	}

	q := trade.NewQuery()
	q.SetTypeFilter("category", cat)
	q.SetTypeFilter("rarity", "nonunique")
	q.Query.Stats = []trade.StatGroup{{Type: "and", Filters: filters}}

	r := &Result{Source: "trade"}
	t := time.Now()
	sr, err := p.Trade.Search(ctx, p.League, q)
	if err != nil {
		return nil, err
	}
	r.Timings = append(r.Timings, Timing{"search", time.Since(t)})
	if sr.Total < 5 && len(filters) > 1 { // one retry: any n−1 of the mods
		n := float64(len(filters) - 1)
		q.Query.Stats = []trade.StatGroup{{Type: "count", Filters: filters, Value: &trade.MinMax{Min: &n}}}
		t = time.Now()
		if sr, err = p.Trade.Search(ctx, p.League, q); err != nil {
			return nil, err
		}
		r.Timings = append(r.Timings, Timing{"search (ослаблен)", time.Since(t)})
		r.Note = fmt.Sprintf("фильтры ослаблены: любые %d из %d модов", len(filters)-1, len(filters))
	}
	r.Total = sr.Total
	r.Link = trade.SearchURL(p.League, sr.ID)

	ids := sr.Result[:min(len(sr.Result), fetchLots)]
	t = time.Now()
	items, err := p.fetchAll(ctx, sr.ID, ids)
	if err != nil {
		return nil, err
	}
	r.Timings = append(r.Timings, Timing{fmt.Sprintf("fetch ×%d", (len(ids)+trade.FetchLimit-1)/trade.FetchLimit), time.Since(t)})

	var lots []fixers.Lot
	for _, fi := range items {
		c, ok := p.toChaos(fi.Listing)
		if !ok {
			continue
		}
		lots = append(lots, fixers.Lot{
			Account:   fi.Listing.Account.Name,
			Signature: signature(fi),
			Chaos:     c,
			Indexed:   fi.Listing.Indexed,
		})
	}
	cr := fixers.Clean(lots, p.Fixers, p.Now())
	r.Kept, r.Dropped, r.Low, r.Mid, r.Chaos = cr.Kept, cr.Dropped, cr.Low, cr.Mid, cr.Mid
	return r, nil
}

// fetchAll runs the fetch requests for all ids in parallel, FetchLimit ids each.
func (p *Pricer) fetchAll(ctx context.Context, queryID string, ids []string) ([]trade.FetchItem, error) {
	type part struct {
		i     int
		items []trade.FetchItem
		err   error
	}
	n := (len(ids) + trade.FetchLimit - 1) / trade.FetchLimit
	ch := make(chan part, n)
	for i := 0; i < n; i++ {
		chunk := ids[i*trade.FetchLimit : min(len(ids), (i+1)*trade.FetchLimit)]
		go func() {
			items, err := p.Trade.Fetch(ctx, queryID, chunk)
			ch <- part{i, items, err}
		}()
	}
	parts := make([][]trade.FetchItem, n)
	for range n {
		pt := <-ch
		if pt.err != nil {
			return nil, pt.err
		}
		parts[pt.i] = pt.items
	}
	var out []trade.FetchItem
	for _, pt := range parts {
		out = append(out, pt...)
	}
	return out, nil
}

func (p *Pricer) toChaos(l trade.Listing) (float64, bool) {
	if l.Price == nil || l.Price.Amount <= 0 {
		return 0, false
	}
	name, ok := p.Currencies[l.Price.Currency]
	if !ok {
		return 0, false
	}
	rate, ok := p.Ninja.Chaos(name)
	if !ok {
		return 0, false
	}
	return l.Price.Amount * rate, true
}

type header struct{ class, rarity, name string }

func readHeader(text string) (header, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) < 3 || !strings.HasPrefix(lines[0], "Item Class: ") || !strings.HasPrefix(lines[1], "Rarity: ") {
		return header{}, ErrNotItem
	}
	return header{
		class:  strings.TrimPrefix(lines[0], "Item Class: "),
		rarity: strings.TrimSpace(strings.TrimPrefix(lines[1], "Rarity: ")),
		name:   strings.TrimSpace(lines[2]),
	}, nil
}

// gemVariant builds poe.ninja's gem qualifier: "level/quality" plus "c" when corrupted.
func gemVariant(text string) string {
	level, quality, corrupted := 0, 0, false
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "Level: ") && level == 0: // gem level comes before "Requirements: Level"
			level, _ = strconv.Atoi(strings.Fields(strings.TrimPrefix(l, "Level: "))[0])
		case strings.HasPrefix(l, "Quality: +"):
			quality, _ = strconv.Atoi(strings.TrimSuffix(strings.Fields(strings.TrimPrefix(l, "Quality: +"))[0], "%"))
		case l == "Corrupted":
			corrupted = true
		}
	}
	v := fmt.Sprintf("%d/%d", level, quality)
	if corrupted {
		v += "c"
	}
	return v
}

func signature(fi trade.FetchItem) string {
	parts := []string{fi.Item.Name}
	for _, m := range fi.Item.ExplicitMods {
		parts = append(parts, m.Description)
	}
	return strings.Join(parts, "|")
}
