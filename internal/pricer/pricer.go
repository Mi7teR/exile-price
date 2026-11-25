// Package pricer routes an item to poe.ninja or the trade site and builds
// the answer line.
package pricer

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/Mi7teR/poe-pricecheck/internal/i18n"

	"github.com/Mi7teR/poe-lib/itemtext"
	"github.com/Mi7teR/poe-pricecheck/internal/fixers"
	"github.com/Mi7teR/poe-pricecheck/internal/ninja"
	"github.com/Mi7teR/poe-pricecheck/internal/trade"
)

type Pricer struct {
	League     string
	Leagues    []string // every pc trade league, in the order GGG lists them
	Trade      *trade.Client
	Stats      *trade.Stats
	Currencies map[string]string // trade currency id → full name, from /data/static
	Parser     *itemtext.Parser
	Fixers     fixers.Config
	RollSlack  float64 // filter min = roll × (1 − RollSlack)
	Now        func() time.Time

	ninja atomic.Pointer[ninja.Index]
}

func (p *Pricer) Ninja() *ninja.Index       { return p.ninja.Load() }
func (p *Pricer) SetNinja(idx *ninja.Index) { p.ninja.Store(idx) }

// Divine is the chaos value of a Divine Orb, 0 when unknown.
func (p *Pricer) Divine() float64 {
	d, _ := p.Ninja().Chaos("Divine Orb")
	return d
}

type Result struct {
	Item     string  // what was priced, for the log
	Source   string  // "poe.ninja" or "trade"
	Low, Mid float64 // chaos; equal for poe.ninja
	Base     float64 // chaos, poe.ninja price of the bare base; 0 = unknown
	Avg      float64 // chaos, poe.ninja price of a unique priced on trade by its rolls
	Total    int     // trade: matches on the site
	Kept     []fixers.Lot
	Dropped  []fixers.Dropped
	Link     string
	Note     string
	Relaxed  bool     // trade: matched any n−1 of the n mods, not all
	Searched []string // rare: what the search asked for, e.g. "68 life"
	Timings  []Timing
}

type Timing struct {
	Step string
	Took time.Duration
}

var ErrNotItem = errors.New("pricer: text is not a PoE item")

// Unsupported says the item kind is not priced in this version.
type Unsupported struct{ What string }

func (e *Unsupported) Error() string { return i18n.Tf("%s is not priced yet", e.What) }

// Price prices one item text (Ctrl+C or Ctrl+Alt+C).
func (p *Pricer) Price(ctx context.Context, text string) (*Result, error) {
	h, err := readHeader(text)
	if err != nil {
		return nil, err
	}
	switch {
	case h.rarity == "Currency" || h.rarity == "Divination Card":
		l, ok := p.Ninja().Best(h.name)
		return p.fromNinja(h.name, l, ok)
	case h.rarity == "Gem":
		level, quality, corrupted := gemProps(text)
		l, ok := p.Ninja().Gem(h.name, level, quality, corrupted)
		return p.fromNinja(h.name, l, ok)
	case h.class == "Maps":
		return p.mapItem(ctx, h)
	case h.rarity == "Normal" || h.rarity == "Magic":
		return p.baseOnly(text, h)
	}

	t0 := time.Now()
	it, err := p.Parser.Parse(text)
	if err != nil {
		return nil, err
	}
	parsed := Timing{"parse", time.Since(t0)}
	var r *Result
	switch {
	case it.Rarity == itemtext.Unique:
		r, err = p.unique(ctx, it, h.base)
	case it.Rarity == itemtext.Rare && isCluster(it):
		r, err = p.cluster(ctx, it)
	case it.Rarity == itemtext.Rare:
		r, err = p.rare(ctx, it, text)
	default:
		return nil, &Unsupported{strings.ToLower(string(it.Rarity)) + " " + it.ItemClass}
	}
	if r != nil {
		r.Timings = append([]Timing{parsed}, r.Timings...)
	}
	return r, err
}

func (p *Pricer) fromNinja(name string, l ninja.Line, ok bool) (*Result, error) {
	if !ok {
		return nil, errors.New(i18n.Tf("%s: not on poe.ninja", name))
	}
	r := &Result{Item: name, Source: "poe.ninja", Low: l.Chaos, Mid: l.Chaos}
	if l.Variant != "" {
		r.Note = l.Variant
	}
	return r, nil
}

// mapItem: unique and special maps come from poe.ninja by name; regular maps
// read "Map (Tier 16)" (one base for every tier) and are priced on the trade
// site by tier and rarity.
func (p *Pricer) mapItem(ctx context.Context, h header) (*Result, error) {
	if h.rarity == "Unique" {
		l, ok := p.Ninja().Best(h.name)
		return p.fromNinja(h.name, l, ok)
	}
	tier := 0
	for _, line := range []string{h.name, h.base} {
		if m := mapTierRe.FindStringSubmatch(line); m != nil {
			tier, _ = strconv.Atoi(m[1])
		}
	}
	if tier == 0 { // a named map such as "Nightmare Map"
		l, ok := p.Ninja().Best(h.name)
		return p.fromNinja(h.name, l, ok)
	}
	q := trade.NewQuery()
	q.Query.Type = trade.Discriminated{Option: "Map", Discriminator: "map"}
	q.SetFilter("type_filters", "rarity", trade.Option{Option: strings.ToLower(h.rarity)})
	q.SetFilter("map_filters", "map_tier", trade.Range(float64(tier), float64(tier)))
	r, err := p.tradePrice(ctx, q, nil)
	if r != nil {
		r.Item = fmt.Sprintf("%s map, tier %d", strings.ToLower(h.rarity), tier)
	}
	return r, err
}

var mapTierRe = regexp.MustCompile(`\bMap \(Tier (\d+)\)`)

var categories = map[string]string{
	"Boots": "armour.boots", "Gloves": "armour.gloves", "Helmets": "armour.helmet",
	"Body Armours": "armour.chest", "Shields": "armour.shield", "Quivers": "armour.quiver",
	"Rings": "accessory.ring", "Amulets": "accessory.amulet", "Belts": "accessory.belt",
	"Jewels": "jewel.base", "Abyss Jewels": "jewel.abyss",

	"Bows": "weapon.bow", "Claws": "weapon.claw", "Daggers": "weapon.basedagger",
	"Rune Daggers": "weapon.runedagger", "One Hand Axes": "weapon.oneaxe",
	"One Hand Swords": "weapon.basesword", "Thrusting One Hand Swords": "weapon.rapier",
	"One Hand Maces": "weapon.basemace", "Sceptres": "weapon.sceptre",
	"Staves": "weapon.basestaff", "Warstaves": "weapon.warstaff", "Two Hand Axes": "weapon.twoaxe",
	"Two Hand Maces": "weapon.twomace", "Two Hand Swords": "weapon.twosword", "Wands": "weapon.wand",
}

const fetchLots = 2 * trade.FetchLimit

// rare searches by explicit mods only: implicits come with the base and made
// the spike's first search too strict.
func (p *Pricer) rare(ctx context.Context, it *itemtext.Item, text string) (*Result, error) {
	cat, ok := categories[it.ItemClass]
	if strings.HasSuffix(it.BaseName, " Eye Jewel") { // whatever the copy calls its class
		cat, ok = "jewel.abyss", true
	}
	if !ok {
		return nil, &Unsupported{"rare " + it.ItemClass}
	}
	weapon := strings.HasPrefix(cat, "weapon")
	m := mods{
		local:  weapon || strings.HasPrefix(cat, "armour.") && cat != "armour.quiver",
		weapon: weapon,
		tiers:  tiers(text),
	}
	filters := rareFilters(it.ActiveExplicits(), m, p.Stats, p.RollSlack)
	q := trade.NewQuery()
	q.SetFilter("type_filters", "category", trade.Option{Option: cat})
	q.SetFilter("type_filters", "rarity", trade.Option{Option: "nonunique"})
	dps := false
	var searched []string
	if weapon {
		pdps, edps := weaponDPS(text)
		for _, d := range []struct {
			key, label string
			v          float64
		}{{"pdps", "pDPS", pdps}, {"edps", "eDPS", edps}} {
			if key, v := d.key, d.v; v >= 1 {
				min := math.Floor(v * (1 - p.RollSlack))
				q.SetFilter("weapon_filters", key, trade.MinMax{Min: &min})
				searched = append(searched, fmt.Sprintf("%.0f %s", v, d.label))
				dps = true
			}
		}
	}
	if len(filters) == 0 && !dps {
		return nil, errors.New(i18n.T("no mod matched a trade stat"))
	}
	q.Query.Stats = []trade.StatGroup{{Type: "and", Filters: filters}}
	if must := mustHave(it, p.Stats, p.RollSlack); len(must) > 0 {
		q.Query.Stats = append(q.Query.Stats, trade.StatGroup{Type: "and", Filters: must})
	}
	r, err := p.tradePrice(ctx, q, filters)
	if r != nil {
		r.Item = it.Name + ", " + it.BaseName
		for _, f := range filters {
			r.Searched = append(r.Searched, f.Label)
		}
		r.Searched = append(r.Searched, searched...)
		if l, ok := p.basePrice(it); ok {
			r.Base = l.Chaos
		}
	}
	return r, err
}

// baseOnly prices a normal or magic item as its crafting base.
func (p *Pricer) baseOnly(text string, h header) (*Result, error) {
	unsupported := &Unsupported{strings.ToLower(h.rarity) + " " + h.class}
	it, err := p.Parser.Parse(text)
	if err != nil {
		return nil, unsupported
	}
	l, ok := p.basePrice(it)
	if !ok {
		return nil, unsupported
	}
	return &Result{Item: fmt.Sprintf("%s, ilvl %d", it.BaseName, it.ItemLevel), Source: "poe.ninja",
		Low: l.Chaos, Mid: l.Chaos, Base: l.Chaos, Note: l.Variant}, nil
}

// basePrice is the poe.ninja price of the item's base. Fractured,
// synthesised, corrupted and mirrored bases are worth something else.
func (p *Pricer) basePrice(it *itemtext.Item) (ninja.Line, bool) {
	if it.BaseName == "" || it.Fractured || it.Synthesised || it.Corrupted || it.Mirrored {
		return ninja.Line{}, false
	}
	return p.Ninja().Base(it.BaseName, it.ItemLevel, it.Influences)
}

// tradePrice searches, relaxes once to "any n−1 of the filters" when fewer
// than 5 lots match, fetches the first lots and cleans out fixers.
func (p *Pricer) tradePrice(ctx context.Context, q trade.Query, filters []trade.StatFilter) (*Result, error) {
	r := &Result{Source: "trade"}
	t := time.Now()
	sr, err := p.Trade.Search(ctx, p.League, q)
	if err != nil {
		return nil, err
	}
	r.Timings = append(r.Timings, Timing{"search", time.Since(t)})
	if sr.Total < 5 && len(filters) > 1 {
		n := float64(len(filters) - 1)
		q.Query.Stats[0] = trade.StatGroup{Type: "count", Filters: filters, Value: &trade.MinMax{Min: &n}}
		t = time.Now()
		if sr, err = p.Trade.Search(ctx, p.League, q); err != nil {
			return nil, err
		}
		r.Timings = append(r.Timings, Timing{"search (relaxed)", time.Since(t)})
		r.Note = i18n.Tf("any %d of %d mods", len(filters)-1, len(filters))
		r.Relaxed = true
	}
	r.Total = sr.Total
	r.Link = trade.SearchURL(p.League, sr.ID)
	if len(sr.Result) == 0 {
		return r, nil
	}

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
	r.Kept, r.Dropped, r.Low, r.Mid = cr.Kept, cr.Dropped, cr.Low, cr.Mid
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
	rate, ok := p.Ninja().Chaos(name)
	if !ok {
		return 0, false
	}
	return l.Price.Amount * rate, true
}

func signature(fi trade.FetchItem) string {
	parts := []string{fi.Item.Name, fi.Item.TypeLine}
	for _, m := range fi.Item.ExplicitMods {
		parts = append(parts, m.Description)
	}
	return strings.Join(parts, "|")
}

// links is the size of the largest linked socket group.
func links(sockets []itemtext.Socket) int {
	n := map[int]int{}
	most := 0
	for _, s := range sockets {
		n[s.Group]++
		most = max(most, n[s.Group])
	}
	return most
}

type header struct{ class, rarity, name, base string }

// readHeader reads "Item Class", "Rarity", the name line and, when the next
// line is not a separator, the base line.
func readHeader(text string) (header, error) {
	lines := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	if len(lines) < 3 || !strings.HasPrefix(lines[0], "Item Class: ") || !strings.HasPrefix(lines[1], "Rarity: ") {
		return header{}, ErrNotItem
	}
	h := header{
		class:  strings.TrimSpace(strings.TrimPrefix(lines[0], "Item Class: ")),
		rarity: strings.TrimSpace(strings.TrimPrefix(lines[1], "Rarity: ")),
		name:   strings.TrimSpace(lines[2]),
	}
	if len(lines) > 3 && !strings.HasPrefix(lines[3], "--------") {
		h.base = strings.TrimSpace(lines[3])
	}
	return h, nil
}

// gemProps reads the gem's own level (the first "Level:" line, before
// "Requirements"), quality and corruption.
func gemProps(text string) (level, quality int, corrupted bool) {
	for _, l := range strings.Split(text, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "Level: ") && level == 0:
			level, _ = strconv.Atoi(firstField(strings.TrimPrefix(l, "Level: ")))
		case strings.HasPrefix(l, "Quality: +"):
			quality, _ = strconv.Atoi(strings.TrimSuffix(firstField(strings.TrimPrefix(l, "Quality: +")), "%"))
		case l == "Corrupted":
			corrupted = true
		}
	}
	return level, quality, corrupted
}

func firstField(s string) string {
	if f := strings.Fields(s); len(f) > 0 {
		return f[0]
	}
	return ""
}
