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
