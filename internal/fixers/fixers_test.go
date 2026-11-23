package fixers

import (
	"testing"
	"time"
)

func TestClean(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	old := now.Add(-72 * time.Hour)
	fresh := now.Add(-1 * time.Hour)
	lots := []Lot{
		{"fixer", "a", 5, old},     // cheap and old → dropped
		{"fixer", "a", 5, old},     // same account and item → duplicate
		{"bargain", "b", 5, fresh}, // cheap but fresh → kept
		{"s1", "c", 40, old},
		{"s2", "d", 50, fresh},
		{"s3", "e", 60, old},
	}
	r := Clean(lots, Default, now)

	if r.Reference != 40 {
		t.Errorf("reference = %v, want 40 (median of 5 unique lots)", r.Reference)
	}
	reasons := map[string]int{}
	for _, d := range r.Dropped {
		reasons[d.Reason]++
	}
	if reasons[ReasonDuplicate] != 1 || reasons[ReasonCheapOld] != 1 {
		t.Errorf("dropped = %+v", r.Dropped)
	}
	if len(r.Kept) != 4 {
		t.Fatalf("kept %d lots, want 4", len(r.Kept))
	}
	if r.Mid != 45 {
		t.Errorf("mid = %v, want 45", r.Mid)
	}
}

func TestCleanEmpty(t *testing.T) {
	r := Clean(nil, Default, time.Now())
	if len(r.Kept) != 0 || r.Mid != 0 {
		t.Errorf("got %+v", r)
	}
}

func TestCleanCheapMarketKeepsCheapLots(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	old := now.Add(-72 * time.Hour)
	// spike, 2026-11-23: four 1c lots were dropped at a 4c median
	lots := []Lot{{"a", "1", 1, old}, {"b", "2", 1, old}, {"c", "3", 4, old}, {"d", "4", 5, old}, {"e", "5", 9, old}}
	r := Clean(lots, Default, now)
	if len(r.Dropped) != 0 {
		t.Errorf("dropped %+v in a 4c market", r.Dropped)
	}
}

func TestCleanDropsOldWhenFreshSuffice(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	week := now.Add(-8 * 24 * time.Hour)
	day := now.Add(-24 * time.Hour)
	lots := []Lot{{"a", "1", 1, week}, {"b", "2", 1, week}, {"c", "3", 2, day}, {"d", "4", 2, day}, {"e", "5", 3, day}, {"f", "6", 3, day}, {"g", "7", 4, day}}
	r := Clean(lots, Default, now)
	if len(r.Kept) != 5 || r.Mid != 3 {
		t.Errorf("kept %d, mid %v; want the 5 fresh lots, mid 3", len(r.Kept), r.Mid)
	}
	// too few fresh lots: the old ones stay
	r = Clean(lots[:4], Default, now)
	if len(r.Kept) != 4 {
		t.Errorf("kept %d of 4 with only 2 fresh", len(r.Kept))
	}
}

func TestCleanKeepsACheapMarket(t *testing.T) {
	now := time.Date(2026, 11, 23, 12, 0, 0, 0, time.UTC)
	day := now.Add(-48 * time.Hour)
	// abyss jewel, 2026-11-23: many 3–10c lots under a 20c+ median are the market
	lots := []Lot{{"a", "1", 3, day}, {"b", "2", 5, day}, {"c", "3", 8, day}, {"d", "4", 10, day},
		{"e", "5", 20, day}, {"f", "6", 25, day}, {"g", "7", 30, day}, {"h", "8", 35, day}, {"i", "9", 50, day}}
	if r := Clean(lots, Default, now); len(r.Dropped) != 0 {
		t.Errorf("dropped %+v", r.Dropped)
	}
}
