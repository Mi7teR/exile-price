package pricer

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/Mi7teR/poe-pricecheck/internal/trade"
)

const (
	enchMoveSpeed = "enchant.stat_308396001"  // "#% increased Movement Speed if you haven't been Hit Recently"
	enchArc       = "enchant.stat_3998182656" // "Arc deals #% increased Damage for each time it has Chained"
)

// priceEnchanted prices one item against the recorded trade responses and
// returns the result and the search bodies sent.
func priceEnchanted(t *testing.T, file string) (*Result, string) {
	t.Helper()
	p := offline(t)
	rt := &recordedTrade{}
	p.Trade = &trade.Client{HTTP: &http.Client{Transport: rt}}
	p.Stats, p.League, p.RollSlack = recordedStats(t), "Covens", 0.15
	p.Currencies = map[string]string{"chaos": "Chaos Orb", "divine": "Divine Orb"}
	p.Now = func() time.Time { return time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC) }
	r, err := p.Price(context.Background(), item(t, file))
	if err != nil {
		t.Fatal(err)
	}
	return r, strings.Join(rt.searches, "\n")
}

func TestEnchantSearched(t *testing.T) {
	for _, c := range []struct{ file, stat, label string }{
		{"boots_normal_enchant.txt", enchMoveSpeed, "10% increased Movement Speed if you haven't been Hit Recently"},
		{"boots_rare_enchant.txt", enchMoveSpeed, "10% increased Movement Speed if you haven't been Hit Recently"},
		{"unique_goldrim_enchant.txt", enchArc, "Arc deals 8% increased Damage for each time it has Chained"},
	} {
		r, searches := priceEnchanted(t, c.file)
		if !strings.Contains(searches, `"id":"`+c.stat+`","value":{"min":`) {
			t.Errorf("%s: enchant %s not searched with a min; searches: %s", c.file, c.stat, searches)
		}
		if r.Source != "trade" {
			t.Errorf("%s: source %q, want trade", c.file, r.Source)
		}
		found := false
		for _, s := range r.Searched {
			found = found || s == c.label
		}
		if !found {
			t.Errorf("%s: searched %q, want it to list the enchant", c.file, r.Searched)
		}
	}
}

func TestEnchantedBaseSearch(t *testing.T) {
	// poe.ninja has no price for a plain ilvl 84 Two-Toned Boots: the
	// enchant alone sends the base to trade
	_, searches := priceEnchanted(t, "boots_normal_enchant.txt")
	for _, want := range []string{`"type":"Two-Toned Boots"`, `"ilvl":{"min":84}`} {
		if !strings.Contains(searches, want) {
			t.Errorf("search lacks %s: %s", want, searches)
		}
	}
}

func TestNoEnchantNoTrade(t *testing.T) {
	p := offline(t)
	rt := &recordedTrade{}
	p.Trade = &trade.Client{HTTP: &http.Client{Transport: rt}}
	p.Stats = recordedStats(t)
	if _, err := p.Price(context.Background(), item(t, "unique_goldrim.txt")); err != nil {
		t.Fatal(err)
	}
	if rt.n.Load() != 0 {
		t.Errorf("cheap unique without an enchant sent %d trade requests", rt.n.Load())
	}
}
