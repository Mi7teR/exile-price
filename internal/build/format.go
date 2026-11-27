package build

import (
	"fmt"
	"math"
	"strings"

	"github.com/Mi7teR/exile-price/internal/i18n"
)

// Tone says whether a stat change is good or bad for the build.
type Tone int

const (
	Neutral Tone = iota
	Better
	Worse
)

// Segment is a piece of the delta line with its tone.
type Segment struct {
	Text string
	Tone Tone
}

// Segments splits the delta line: the prefix, one segment per changed stat,
// and a trailing note when not everything was counted.
func (d *Delta) Segments() []Segment {
	prefix := i18n.Tf("instead of %s: ", strings.Join(d.Slots, " + "))
	segs := []Segment{{Text: prefix}}

	type stat struct {
		label, key string
		pct        bool // percent of Before, else absolute
		suffix     string
	}
	dps := dpsKey(d.Before)
	stats := []stat{
		{"DPS", dps, true, ""},
		{"Life", "Life", false, ""},
		{"ES", "EnergyShield", false, ""},
		{"Fire", "FireResist", false, "%"},
		{"Cold", "ColdResist", false, "%"},
		{"Light", "LightningResist", false, "%"},
		{"Chaos", "ChaosResist", false, "%"},
		{"EHP", "TotalEHP", true, ""},
	}
	first := true
	for _, s := range stats {
		c := d.Change[s.key]
		var text string
		var v float64
		if s.pct && d.Before[s.key] > 0 {
			v = c / d.Before[s.key] * 100
			if math.Round(v*10) == 0 {
				continue
			}
			text = fmt.Sprintf("%s %+.1f%%", s.label, v)
		} else {
			v = c
			if math.Round(v) == 0 {
				continue
			}
			text = fmt.Sprintf("%s %+.0f%s", s.label, v, s.suffix)
		}
		tone := Better
		if v < 0 {
			tone = Worse
		}
		if !first {
			text = " · " + text
		}
		first = false
		segs = append(segs, Segment{Text: text, Tone: tone})
	}
	if first {
		segs = append(segs, Segment{Text: i18n.T("no change")})
	}
	if d.Partial {
		segs = append(segs, Segment{Text: i18n.T(" · not all counted")})
	}
	return segs
}

// String is the delta line as plain text, for the log and CLI.
func (d *Delta) String() string {
	var b strings.Builder
	for _, s := range d.Segments() {
		b.WriteString(s.Text)
	}
	return b.String()
}
