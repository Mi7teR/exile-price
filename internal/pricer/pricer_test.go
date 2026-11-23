package pricer

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/Mi7teR/poe-lib/gamedata"
	"github.com/Mi7teR/poe-lib/itemtext"
	"github.com/Mi7teR/poe-pricecheck/internal/fixers"
	"github.com/Mi7teR/poe-pricecheck/internal/ninja"
)

// offline is a pricer over the recorded poe.ninja response, no trade client:
// only the poe.ninja routes can run.
func offline(t *testing.T) *Pricer {
	t.Helper()
	f, err := os.Open("../../testdata/recorded/ninja-overviews.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	idx, err := ninja.Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	gd, err := gamedata.PoE1()
	if err != nil {
		t.Fatal(err)
	}
	p := &Pricer{Parser: itemtext.New(gd)}
	p.SetNinja(idx)
	return p
}

func item(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile("../../testdata/items/" + name)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestPriceFromNinja(t *testing.T) {
	p := offline(t)
	cases := []struct{ file, wantNote string }{
		{"currency_divine.txt", ""},
		{"divcard.txt", ""},
		{"gem_arc.txt", "20/20c"},
		{"real_gem.txt", "21/20c"},
		{"real_unique_6l.txt", "Simple Robe, 6L"},
		{"unique_goldrim.txt", "Leather Cap"},
		{"unique_shavronne_6l.txt", "Occultist's Vestment, 6L"},
	}
	for _, c := range cases {
		r, err := p.Price(context.Background(), item(t, c.file))
		if err != nil {
			t.Errorf("%s: %v", c.file, err)
			continue
		}
		if r.Source != "poe.ninja" || r.Mid <= 0 || r.Note != c.wantNote {
			t.Errorf("%s: got %+v, want poe.ninja with note %q", c.file, r, c.wantNote)
		}
	}
}

func TestPriceRejects(t *testing.T) {
	p := offline(t)
	if _, err := p.Price(context.Background(), "hello"); !errors.Is(err, ErrNotItem) {
		t.Errorf("plain text: %v", err)
	}
	magic := "Item Class: Boots\nRarity: Magic\nSprinter's Leather Boots\n--------\nItem Level: 10\n--------\n10% increased Movement Speed\n"
	var u *Unsupported
	if _, err := p.Price(context.Background(), magic); !errors.As(err, &u) {
		t.Errorf("magic boots: %v", err)
	}
}

func TestMapTier(t *testing.T) {
	for file, want := range map[string]string{"real_map_normal.txt": "16", "real_map_rare.txt": "16"} {
		h, err := readHeader(item(t, file))
		if err != nil {
			t.Fatal(err)
		}
		m := mapTierRe.FindStringSubmatch(h.name)
		if m == nil || m[1] != want || h.class != "Maps" {
			t.Errorf("%s: header %+v", file, h)
		}
	}
}

func TestLine(t *testing.T) {
	r := &Result{Source: "poe.ninja", Mid: 418.8, Low: 418.8}
	if got := r.Line(209.4); got != "~2.0 div · poe.ninja" {
		t.Errorf("ninja line = %q", got)
	}
}

func TestPriceBase(t *testing.T) {
	p := offline(t)
	cases := []struct {
		text, line string
	}{
		{"Item Class: Gloves\nRarity: Normal\nRunic Gauntlets\n--------\nItem Level: 85\n", "base ~4.7c · poe.ninja"},
		{"Item Class: Boots\nRarity: Magic\nZaffre Paladin Boots of the Phoenix\n--------\nItem Level: 86\n--------\nElder Item\n", "base ~30c · poe.ninja"},
	}
	for _, c := range cases {
		r, err := p.Price(context.Background(), c.text)
		if err != nil {
			t.Errorf("%q: %v", c.text, err)
			continue
		}
		if got := r.Line(209.4); got != c.line {
			t.Errorf("%q: line %q, want %q", c.text, got, c.line)
		}
	}
	// a fractured base is worth its fractured mod, not the base price
	var u *Unsupported
	if _, err := p.Price(context.Background(), item(t, "real_boots_magic_fractured.txt")); !errors.As(err, &u) {
		t.Errorf("fractured magic boots: %v", err)
	}
	r := &Result{Source: "trade", Low: 14, Mid: 15, Base: 4.7, Kept: make([]fixers.Lot, 7)}
	if got := r.Line(209.4); got != "~14c–15c · 7 listings · base ~4.7c" {
		t.Errorf("rare line with base = %q", got)
	}
}

func TestAutoLeague(t *testing.T) {
	// order and names from the recorded /api/trade/data/leagues, pc realm
	ids := []string{"Covens", "Hardcore Covens", "Ruthless Covens", "HC Ruthless Covens", "Standard", "Hardcore", "Ruthless", "Hardcore Ruthless"}
	if got := AutoLeague(ids); got != "Covens" {
		t.Errorf("auto = %q", got)
	}
	if got := AutoLeague(ids[1:]); got != "" {
		t.Errorf("no temporary softcore league, got %q", got)
	}
}
