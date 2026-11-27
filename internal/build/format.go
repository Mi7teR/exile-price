package build

import (
	"fmt"
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
	if d.Empty {
		prefix = i18n.Tf("into empty %s: ", strings.Join(d.Slots, " + "))
	}
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
		v, num, suffix := c, fmt.Sprintf("%+.0f", c), s.suffix
		if s.pct && d.Before[s.key] > 0 {
			v = c / d.Before[s.key] * 100
			num, suffix = fmt.Sprintf("%+.1f", v), "%"
		}
		if isZero(num) { // decided on the printed number: %+.0f of 0.5 is "+0"
			continue
		}
		text := s.label + " " + num + suffix
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

// isZero reports whether a formatted number such as "+0", "-0.0" is zero.
func isZero(num string) bool {
	return strings.Trim(num, "+-0.") == ""
}

// String is the delta line as plain text, for the log and CLI.
func (d *Delta) String() string {
	var b strings.Builder
	for _, s := range d.Segments() {
		b.WriteString(s.Text)
	}
	return b.String()
}
