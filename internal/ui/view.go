// Package ui draws the answer line with Gio. It holds no platform code, so it
// builds and its layout runs on any OS.
package ui

import (
	"image"
	"image/color"
	"sync"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

type Kind int

const (
	Price   Kind = iota // a price
	Pending             // waiting for the trade site
	Info                // startup, hints
	Error
	Notice // a live search found a listing
	Better // a stat got better
	Worse  // a stat got worse
	Muted  // secondary text
)

var colors = map[Kind]color.NRGBA{
	Price:   {R: 236, G: 214, B: 150, A: 255},
	Pending: {R: 160, G: 160, B: 170, A: 255},
	Info:    {R: 170, G: 200, B: 230, A: 255},
	Error:   {R: 240, G: 130, B: 120, A: 255},
	Notice:  {R: 140, G: 230, B: 140, A: 255},
	Better:  {R: 140, G: 230, B: 140, A: 255},
	Worse:   {R: 240, G: 130, B: 120, A: 255},
	Muted:   {R: 160, G: 160, B: 170, A: 255},
}

var background = color.NRGBA{R: 18, G: 18, B: 22, A: 245}

// Span is a run of the second line in one color.
type Span struct {
	Text string
	Kind Kind
}

// View is the answer line, with an optional second line under it. Set may be called from any goroutine.
type View struct {
	th   *material.Theme
	mu   sync.Mutex
	text string
	kind Kind
	sub  []Span
}

func New() *View { return &View{th: material.NewTheme()} }

func (v *View) Set(text string, kind Kind) {
	v.mu.Lock()
	v.text, v.kind = text, kind
	v.mu.Unlock()
}

// SetSub sets the second line; nil removes it. Set leaves it alone.
func (v *View) SetSub(spans []Span) {
	v.mu.Lock()
	v.sub = spans
	v.mu.Unlock()
}

// Layout draws the line and returns the window size it needs, in pixels.
func (v *View) Layout(gtx layout.Context) image.Point {
	v.mu.Lock()
	text, kind, sub := v.text, v.kind, v.sub
	v.mu.Unlock()

	// Measure on an unconstrained single line, then paint the background
	// under it and replay the recorded text.
	m := op.Record(gtx.Ops)
	measure := gtx
	measure.Constraints = layout.Constraints{Max: image.Pt(1<<14, 1<<14)}
	line := func(text string, kind Kind) layout.FlexChild {
		return layout.Rigid(func(gtx layout.Context) layout.Dimensions {
			l := material.Body1(v.th, text)
			l.MaxLines = 1
			l.Color = colors[kind]
			return l.Layout(gtx)
		})
	}
	dims := layout.UniformInset(unit.Dp(8)).Layout(measure, func(gtx layout.Context) layout.Dimensions {
		children := []layout.FlexChild{line(text, kind)}
		if len(sub) > 0 {
			children = append(children, layout.Rigid(func(gtx layout.Context) layout.Dimensions {
				spans := make([]layout.FlexChild, len(sub))
				for i, sp := range sub {
					spans[i] = line(sp.Text, sp.Kind)
				}
				return layout.Flex{}.Layout(gtx, spans...)
			}))
		}
		return layout.Flex{Axis: layout.Vertical}.Layout(gtx, children...)
	})
	call := m.Stop()
	paint.FillShape(gtx.Ops, background, clip.Rect{Max: gtx.Constraints.Max}.Op())
	call.Add(gtx.Ops)
	return dims.Size
}
