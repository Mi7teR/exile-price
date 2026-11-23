package ui

import (
	"image/color"
	"strings"

	"github.com/Mi7teR/poe-pricecheck/internal/i18n"

	"gioui.org/font"
	"gioui.org/layout"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"
)

// Auto is the league choice "pick the current temporary softcore league".
const Auto = ""

type Action int

const (
	None Action = iota
	Saved
	Cancelled
)

// Settings is the form for the league and the POESESSID.
type Settings struct {
	th      *material.Theme
	leagues []string
	auto    string // what Auto resolves to now, shown next to it

	league       widget.Enum
	sess         widget.Editor
	show         widget.Bool
	save, cancel widget.Clickable
	list         widget.List
}

const autoKey = "\x00auto"

// NewSettings opens the form on the current choice: league is Auto or a
// league id; auto is the league Auto picks today.
func NewSettings(leagues []string, auto, league, poesessid string) *Settings {
	s := &Settings{th: material.NewTheme(), leagues: leagues, auto: auto}
	s.league.Value = autoKey
	if league != Auto {
		s.league.Value = league
		if !contains(leagues, league) { // set by hand in config.toml, or an ended league
			s.leagues = append([]string{league}, leagues...)
		}
	}
	s.sess.SingleLine = true
	s.sess.Submit = true
	s.sess.Mask = '•'
	s.sess.SetText(poesessid)
	s.list.Axis = layout.Vertical
	return s
}

// Values is what the form holds now: the league (Auto or an id) and the
// trimmed POESESSID.
func (s *Settings) Values() (league, poesessid string) {
	league = s.league.Value
	if league == autoKey {
		league = Auto
	}
	return league, strings.TrimSpace(s.sess.Text())
}

// Problem explains why the form cannot be saved, or is empty.
func (s *Settings) Problem() string {
	_, sess := s.Values()
	if sess != "" && !validSession(sess) {
		return i18n.T("POESESSID is 32 characters 0–9 and a–f")
	}
	return ""
}

func validSession(s string) bool {
	if len(s) != 32 {
		return false
	}
	for _, r := range s {
		if !(r >= '0' && r <= '9' || r >= 'a' && r <= 'f' || r >= 'A' && r <= 'F') {
			return false
		}
	}
	return true
}

var (
	formBg = color.NRGBA{R: 250, G: 250, B: 252, A: 255}
	muted  = color.NRGBA{R: 100, G: 100, B: 110, A: 255}
	bad    = color.NRGBA{R: 190, G: 40, B: 30, A: 255}
)

// Layout draws the form and reports Saved or Cancelled once a button is pressed.
func (s *Settings) Layout(gtx layout.Context) Action {
	s.league.Update(gtx)
	if s.show.Update(gtx) {
		s.sess.Mask = '•'
		if s.show.Value {
			s.sess.Mask = 0
		}
	}
	problem := s.Problem()
	act := None
	if s.cancel.Clicked(gtx) {
		act = Cancelled
	}
	if s.save.Clicked(gtx) && problem == "" {
		act = Saved
	}

	paint.FillShape(gtx.Ops, formBg, clip.Rect{Max: gtx.Constraints.Max}.Op())
	th := s.th
	gap := layout.Spacer{Height: unit.Dp(12)}.Layout
	small := func(text string, c color.NRGBA) layout.Widget {
		l := material.Caption(th, text)
		l.Color = c
		return l.Layout
	}

	heading := func(text string) layout.Widget {
		l := material.Subtitle1(th, text)
		l.Font.Weight = font.Bold
		return l.Layout
	}
	rows := []layout.Widget{heading(i18n.T("League"))}
	autoLabel := i18n.T("Auto: the current temporary softcore league")
	if s.auto != "" {
		autoLabel += i18n.Tf(" (now %s)", s.auto)
	}
	rows = append(rows, material.RadioButton(th, &s.league, autoKey, autoLabel).Layout)
	for _, lg := range s.leagues {
		rows = append(rows, material.RadioButton(th, &s.league, lg, lg).Layout)
	}
	rows = append(rows,
		gap,
		heading("POESESSID"),
		small(i18n.T("Optional: the trade site works without a login today. Where to get it: pathofexile.com → F12 → Application → Cookies → POESESSID. Stored in config.toml, never written to the log."), muted),
		layout.Spacer{Height: unit.Dp(6)}.Layout,
		func(gtx layout.Context) layout.Dimensions {
			e := material.Editor(th, &s.sess, i18n.T("empty — no login"))
			e.Font.Typeface = "monospace"
			return widget.Border{Color: muted, CornerRadius: unit.Dp(4), Width: unit.Dp(1)}.Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				return layout.UniformInset(unit.Dp(8)).Layout(gtx, e.Layout)
			})
		},
		material.CheckBox(th, &s.show, i18n.T("Show")).Layout,
	)
	if problem != "" {
		rows = append(rows, small(problem, bad))
	}
	rows = append(rows, gap, func(gtx layout.Context) layout.Dimensions {
		return layout.Flex{Spacing: layout.SpaceStart}.Layout(gtx,
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				b := material.Button(th, &s.cancel, i18n.T("Cancel"))
				b.Background = color.NRGBA{R: 225, G: 225, B: 230, A: 255}
				b.Color = color.NRGBA{R: 30, G: 30, B: 35, A: 255}
				return b.Layout(gtx)
			}),
			layout.Rigid(layout.Spacer{Width: unit.Dp(8)}.Layout),
			layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				b := material.Button(th, &s.save, i18n.T("Save"))
				b.Font.Weight = font.Bold
				b.Background = color.NRGBA{R: 40, G: 90, B: 170, A: 255}
				if problem != "" {
					gtx = gtx.Disabled()
				}
				return b.Layout(gtx)
			}),
		)
	})

	layout.UniformInset(unit.Dp(16)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
		return material.List(th, &s.list).Layout(gtx, len(rows), func(gtx layout.Context, i int) layout.Dimensions {
			return rows[i](gtx)
		})
	})
	return act
}

func contains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}
