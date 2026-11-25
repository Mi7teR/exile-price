package pricer

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/Mi7teR/poe-lib/gamedata"
	"github.com/Mi7teR/poe-lib/itemtext"
	"github.com/Mi7teR/poe-pricecheck/internal/trade"
)

func recordedStats(t *testing.T) *trade.Stats {
	t.Helper()
	f, err := os.Open("../../testdata/recorded/data-stats.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	s, err := trade.ParseStats(f)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func filterMins(t *testing.T, file string) map[string]float64 {
	t.Helper()
	gd, err := gamedata.PoE1()
	if err != nil {
		t.Fatal(err)
	}
	it, err := itemtext.New(gd).Parse(item(t, file))
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]float64{}
	m := mods{local: true, tiers: tiers(item(t, file))}
	for _, f := range rareFilters(it.ActiveExplicits(), m, recordedStats(t), 0.15) {
		got[f.ID] = *f.Value.Min
	}
	return got
}

func TestRareFiltersPseudo(t *testing.T) {
	got := filterMins(t, "boots_rare.txt")
	// +68 life; 38 cold + 41 lightning = 79 elemental; implicit not counted
	want := map[string]float64{pseudoLife: 57, pseudoElemRes: 67}
	for id, v := range want {
		if got[id] != v {
			t.Errorf("%s min = %v, want %v (all: %v)", id, got[id], v, got)
		}
	}
	if len(got) != 3 { // + movement speed
		t.Errorf("filters = %v, want life, elemental res and movement speed", got)
	}
}

func filterLabels(t *testing.T, file string, m mods) []string {
	t.Helper()
	gd, err := gamedata.PoE1()
	if err != nil {
		t.Fatal(err)
	}
	text := item(t, file)
	it, err := itemtext.New(gd).Parse(text)
	if err != nil {
		t.Fatal(err)
	}
	m.tiers = tiers(text)
	var got []string
	for _, f := range rareFilters(it.ActiveExplicits(), m, recordedStats(t), 0.15) {
		got = append(got, f.Label)
	}
	return got
}

func TestRareFilterLabels(t *testing.T) {
	got := filterLabels(t, "boots_rare.txt", mods{local: true})
	want := "[68 life 79% res 30% increased Movement Speed]"
	if fmt.Sprint(got) != want {
		t.Errorf("labels = %v, want %s", got, want)
	}
}

func TestRareFiltersWeaponNoJunk(t *testing.T) {
	// every mod is in the DPS filters, low tier or junk: the bow is searched
	// by its DPS alone, not by accuracy, life per kill and dexterity
	if got := filterLabels(t, "real_bow_rare_advanced.txt", mods{local: true, weapon: true}); len(got) != 0 {
		t.Errorf("bow filters = %v, want none", got)
	}
}

func TestRareFiltersSkipCrafted(t *testing.T) {
	// "+37 to maximum Mana" is a bench craft: anyone adds it for a few chaos
	got := filterLabels(t, "real_ring_rare_crafted.txt", mods{})
	want := "[+36 to maximum Energy Shield Adds 12 to 23 Cold Damage to Attacks 5% increased Attack Speed]"
	if fmt.Sprint(got) != want {
		t.Errorf("ring labels = %v, want %s", got, want)
	}
}

func TestRareFiltersSkipJunkAndLowTiers(t *testing.T) {
	// ward T5, rarity T2, life per kill T1 (junk), dexterity T6: only rarity counts
	got := filterMins(t, "real_gloves_rare_advanced.txt")
	if len(got) != 1 || got["explicit.stat_3917489142"] != 8 {
		t.Errorf("filters = %v, want rarity ≥ 8 alone", got)
	}
}

func TestTiers(t *testing.T) {
	got := tiers(item(t, "real_gloves_rare_advanced.txt"))
	want := map[string]int{"54% increased Ward": 5, "+10% to Item Rarity": 2, "Gain 108 Life per Enemy Killed": 1, "+32 to Dexterity": 6}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("tier of %q = %d, want %d (all %v)", k, got[k], v, got)
		}
	}
}

func TestWeaponDPS(t *testing.T) {
	text := "Physical Damage: 100-200 (augmented)\nElemental Damage: 10-20 (augmented), 30-50 (augmented)\nAttacks per Second: 1.50 (augmented)\n"
	p, e := weaponDPS(text)
	if p != 225 || e != 82.5 {
		t.Errorf("dps = %v, %v; want 225, 82.5", p, e)
	}
}

func TestLocalStat(t *testing.T) {
	st, local, ok := mods{local: true}.resolve(recordedStats(t), "+50 to Armour")
	if !ok || !local || st.ID == "" {
		t.Errorf("armour on boots: %+v local=%v ok=%v", st, local, ok)
	}
	if _, local, _ := (mods{}).resolve(recordedStats(t), "+50 to Armour"); local {
		t.Error("armour on a ring resolved as local")
	}
}

func TestResistances(t *testing.T) {
	cases := []struct {
		text        string
		elem, chaos float64
		ok          bool
	}{
		{"+38% to Cold Resistance", 38, 0, true},
		{"+12% to Fire and Lightning Resistances", 24, 0, true},
		{"+10% to all Elemental Resistances", 30, 0, true},
		{"+15% to Cold and Chaos Resistances", 15, 15, true},
		{"+23% to Chaos Resistance", 0, 23, true},
		{"+9(6-10)% to Fire Resistance", 9, 0, true},
		{"+1% to maximum Fire Resistance", 0, 0, false},
	}
	for _, c := range cases {
		e, ch, ok := resistances(c.text)
		if e != c.elem || ch != c.chaos || ok != c.ok {
			t.Errorf("%q = %v, %v, %v", c.text, e, ch, ok)
		}
	}
}

func TestInDPS(t *testing.T) {
	for text, want := range map[string]bool{
		"Adds 15 to 45 Physical Damage":             true,
		"Adds 3 to 60 Lightning Damage":             true,
		"112% increased Physical Damage":            true,
		"13% increased Attack Speed":                true,
		"Adds 10 to 20 Fire Damage to Spells":       false,
		"Adds 5 to 9 Chaos Damage":                  false,
		"40% increased Spell Damage":                false,
		"+31% to Global Critical Strike Multiplier": false,
	} {
		if inDPSRe.MatchString(text) != want {
			t.Errorf("%q in DPS = %v", text, !want)
		}
	}
}

func parsed(t *testing.T, file string) *itemtext.Item {
	t.Helper()
	gd, err := gamedata.PoE1()
	if err != nil {
		t.Fatal(err)
	}
	it, err := itemtext.New(gd).Parse(item(t, file))
	if err != nil {
		t.Fatal(err)
	}
	return it
}

func TestClusterStats(t *testing.T) {
	it := parsed(t, "real_cluster_medium.txt")
	if !isCluster(it) {
		t.Fatalf("not a cluster: %q", it.BaseName)
	}
	// ids from a real listing, 2026-11-23
	base, notables := clusterFilters(it, recordedStats(t))
	ids := []string{}
	for _, f := range append(base, notables...) {
		id := f.ID
		if f.Value != nil && f.Value.Option != nil {
			id += "|opt"
		}
		ids = append(ids, id)
	}
	want := "[enchant.stat_3086156145 enchant.stat_4079888060 enchant.stat_3948993189|opt explicit.stat_2783012144]"
	if got := fmt.Sprint(ids); got != want {
		t.Errorf("filters = %s, want %s", got, want)
	}
	if *base[0].Value.Min != 4 || *base[0].Value.Max != 4 {
		t.Errorf("passives = %+v, want exactly 4", base[0].Value)
	}
	if f := trade.Exact("enchant.stat_3948993189|32"); f.ID != "enchant.stat_3948993189" || f.Value.Option != 32 {
		t.Errorf("option filter = %+v", f)
	}
}

func TestUniqueFilters(t *testing.T) {
	it := &itemtext.Item{Explicits: []itemtext.ModLine{
		{Text: "+31 to maximum Life", HasRange: true},
		{Text: "+48% to Lightning Resistance", HasRange: true},
		{Text: "15% reduced Mana Reservation Efficiency of Skills", HasRange: true},
		{Text: "Has 1 Socket"},
	}}
	got := uniqueFilters(it, mods{}, recordedStats(t))
	if len(got) != 2 || *got[0].Value.Min != 29 || *got[1].Value.Min != 45 {
		t.Errorf("filters = %+v, want life ≥ 29 and lightning ≥ 45", got)
	}
}

func TestSocketFilter(t *testing.T) {
	gd, err := gamedata.PoE1()
	if err != nil {
		t.Fatal(err)
	}
	six := strings.Replace(item(t, "boots_rare.txt"), "Sockets: B-R-R-G", "Sockets: B-R G R-G B", 1)
	offSix := strings.Replace(item(t, "boots_rare.txt"), "Sockets: B-R-R-G", "Sockets: G-G G-G B R", 1)
	cases := []struct{ text, label, json string }{
		// a dex bow: its three white sockets are off-colour, the greens are not
		{item(t, "real_bow_rare_advanced.txt"), "5L 3W", `"socket_filters":{"filters":{"links":{"min":5,"w":3}}}`},
		{item(t, "boots_rare.txt"), "", ""}, // a 4-link is a few fusings
		// str/int boots: two greens of six is not mostly off-colour
		{six, "6S", `"socket_filters":{"filters":{"sockets":{"min":6}}}`},
		// four greens of six is
		{offSix, "6S 4G", `"socket_filters":{"filters":{"sockets":{"min":6,"g":4}}}`},
	}
	for _, c := range cases {
		it, err := itemtext.New(gd).Parse(c.text)
		if err != nil {
			t.Fatal(err)
		}
		q := trade.NewQuery()
		label := socketFilter(&q, it)
		b, _ := json.Marshal(q)
		if label != c.label || c.json != "" && !strings.Contains(string(b), c.json) || c.json == "" && strings.Contains(string(b), "socket_filters") {
			t.Errorf("label %q, query %s; want %q with %s", label, b, c.label, c.json)
		}
	}
}
