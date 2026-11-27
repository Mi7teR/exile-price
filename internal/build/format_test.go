package build

import (
	"testing"

	"github.com/Mi7teR/exile-price/internal/i18n"
)

func setLang(t *testing.T, l i18n.Lang) {
	t.Helper()
	prev := i18n.Current()
	i18n.Set(l)
	t.Cleanup(func() { i18n.Set(prev) })
}

func ringDelta() *Delta {
	return &Delta{
		Slots:  []string{"Ring 2"},
		Before: map[string]float64{"CombinedDPS": 3158415, "TotalEHP": 108378},
		Change: map[string]float64{"CombinedDPS": -439548, "Life": -49, "EnergyShield": 37, "FireResist": 4, "TotalEHP": 613},
	}
}

func TestSegmentsRing(t *testing.T) {
	setLang(t, i18n.RU)
	d := ringDelta()
	want := "вместо Ring 2: DPS -13.9% · Life -49 · ES +37 · Fire +4% · EHP +0.6%"
	if got := d.String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	tones := []Tone{Neutral, Worse, Worse, Better, Better, Better}
	segs := d.Segments()
	if len(segs) != len(tones) {
		t.Fatalf("got %d segments: %+v", len(segs), segs)
	}
	for i, s := range segs {
		if s.Tone != tones[i] {
			t.Errorf("segment %d %q: tone %v, want %v", i, s.Text, s.Tone, tones[i])
		}
	}
}

func TestSegmentsEnglish(t *testing.T) {
	setLang(t, i18n.EN)
	want := "instead of Ring 2: DPS -13.9% · Life -49 · ES +37 · Fire +4% · EHP +0.6%"
	if got := ringDelta().String(); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSegmentsFullDPSPreferred(t *testing.T) {
	setLang(t, i18n.EN)
	d := &Delta{Slots: []string{"Ring 2"},
		Before: map[string]float64{"FullDPS": 1000, "CombinedDPS": 500},
		Change: map[string]float64{"FullDPS": 100, "CombinedDPS": -50}}
	if got, want := d.String(), "instead of Ring 2: DPS +10.0%"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSegmentsZeroBefore(t *testing.T) {
	setLang(t, i18n.EN)
	d := &Delta{Slots: []string{"Ring 2"},
		Before: map[string]float64{"CombinedDPS": 0},
		Change: map[string]float64{"CombinedDPS": 1234}}
	if got, want := d.String(), "instead of Ring 2: DPS +1234"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSegmentsTwoHander(t *testing.T) {
	setLang(t, i18n.RU)
	d := &Delta{Slots: []string{"Weapon 1", "Weapon 2"}}
	segs := d.Segments()
	if got, want := segs[0].Text, "вместо Weapon 1 + Weapon 2: "; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSegmentsRoundsAwayNoise(t *testing.T) {
	d := &Delta{Slots: []string{"Ring 2"},
		Before: map[string]float64{"TotalEHP": 108378},
		Change: map[string]float64{"Life": 0.3, "TotalEHP": 0.01}}
	setLang(t, i18n.RU)
	if got, want := d.String(), "вместо Ring 2: без изменений"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	setLang(t, i18n.EN)
	if got, want := d.String(), "instead of Ring 2: no change"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSegmentsPartial(t *testing.T) {
	d := ringDelta()
	d.Partial = true
	setLang(t, i18n.RU)
	segs := d.Segments()
	last := segs[len(segs)-1]
	if last.Text != " · не всё учтено" || last.Tone != Neutral {
		t.Fatalf("last segment %+v", last)
	}
	setLang(t, i18n.EN)
	if got := d.String(); got[len(got)-len(" · not all counted"):] != " · not all counted" {
		t.Fatalf("got %q", got)
	}
}

func TestSegmentsEmptySlot(t *testing.T) {
	d := &Delta{Slots: []string{"Ring 2"}, Empty: true,
		Change: map[string]float64{"Life": 40}}
	setLang(t, i18n.EN)
	if got, want := d.String(), "into empty Ring 2: Life +40"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
	setLang(t, i18n.RU)
	if got, want := d.String(), "в пустой Ring 2: Life +40"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestSegmentsHalfRoundsToZero(t *testing.T) {
	setLang(t, i18n.EN)
	for _, v := range []float64{0.5, -0.5} {
		d := &Delta{Slots: []string{"Ring 2"},
			Before: map[string]float64{"TotalEHP": 1000},
			Change: map[string]float64{"Life": v, "FireResist": v, "TotalEHP": v / 2}}
		if got, want := d.String(), "instead of Ring 2: no change"; got != want {
			t.Errorf("%v: got %q, want %q", v, got, want)
		}
	}
}
