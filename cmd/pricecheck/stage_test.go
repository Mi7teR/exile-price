package main

import (
	"testing"
	"time"
)

func TestStageText(t *testing.T) {
	for _, c := range []struct {
		step string
		wait time.Duration
		want string
	}{
		{"search", 0, "searching trade…"},
		{"fetch", 0, "reading listings…"},
		{"search", 3200 * time.Millisecond, "waiting for the trade limit · 4 s…"},
		{"fetch", 300 * time.Millisecond, "waiting for the trade limit · 1 s…"},
	} {
		if got := stageText(c.step, c.wait); got != c.want {
			t.Errorf("stageText(%q, %v) = %q, want %q", c.step, c.wait, got, c.want)
		}
	}
}

func TestStageStopsWhenDone(t *testing.T) {
	var shown []string
	s := stages{show: func(text string) { shown = append(shown, text) }}
	s.report("search", 0)
	s.stop()
	s.report("fetch", 0) // a fetch still running after the price failed
	if len(shown) != 1 || shown[0] != "searching trade…" {
		t.Errorf("shown = %q", shown)
	}
}
