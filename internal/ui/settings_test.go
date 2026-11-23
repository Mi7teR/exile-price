package ui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

var leagues = []string{"Covens", "Hardcore Covens", "Standard"}

func TestSettingsValues(t *testing.T) {
	s := NewSettings(leagues, "Covens", Auto, "")
	if lg, sess := s.Values(); lg != Auto || sess != "" {
		t.Errorf("auto, no session: got %q %q", lg, sess)
	}
	s = NewSettings(leagues, "Covens", "Hardcore Covens", " 0123456789ABCDEF0123456789abcdef ")
	if lg, sess := s.Values(); lg != "Hardcore Covens" || sess != "0123456789ABCDEF0123456789abcdef" {
		t.Errorf("chosen league, trimmed session: got %q %q", lg, sess)
	}
	if p := s.Problem(); p != "" {
		t.Errorf("valid session flagged: %q", p)
	}
}

func TestSettingsRejectsBadSession(t *testing.T) {
	for _, bad := range []string{"abc", "0123456789abcdef0123456789abcdeg", "POESESSID=0123456789abcdef0123456789abcdef"} {
		if NewSettings(leagues, "Covens", Auto, bad).Problem() == "" {
			t.Errorf("%q accepted", bad)
		}
	}
}

func TestSettingsKeepsHandSetLeague(t *testing.T) {
	s := NewSettings(leagues, "Covens", "Settlers", "")
	if lg, _ := s.Values(); lg != "Settlers" {
		t.Errorf("league from config.toml lost: %q", lg)
	}
	if s.leagues[0] != "Settlers" {
		t.Errorf("hand-set league not offered: %v", s.leagues)
	}
}

func TestSettingsLayoutRuns(t *testing.T) {
	s := NewSettings(leagues, "Covens", Auto, "")
	var ops op.Ops
	gtx := layout.Context{Ops: &ops, Metric: unit.Metric{PxPerDp: 1, PxPerSp: 1}, Constraints: layout.Exact(image.Pt(460, 560))}
	if act := s.Layout(gtx); act != None {
		t.Errorf("no click, got action %v", act)
	}
}
