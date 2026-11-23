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
