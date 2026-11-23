package pricer

import (
	"os"
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
	for _, f := range rareFilters(it.ActiveExplicits(), recordedStats(t), 0.15) {
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

func TestRareFiltersSkipJunk(t *testing.T) {
	got := filterMins(t, "real_gloves_rare_advanced.txt")
	if len(got) != 3 {
		t.Errorf("filters = %v, want ward, rarity and dexterity without life per kill", got)
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
