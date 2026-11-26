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
)

var colors = map[Kind]color.NRGBA{
	Price:   {R: 236, G: 214, B: 150, A: 255},
	Pending: {R: 160, G: 160, B: 170, A: 255},
	Info:    {R: 170, G: 200, B: 230, A: 255},
	Error:   {R: 240, G: 130, B: 120, A: 255},
	Notice:  {R: 140, G: 230, B: 140, A: 255},
}

var background = color.NRGBA{R: 18, G: 18, B: 22, A: 245}

// View is the one-line answer. Set may be called from any goroutine.
type View struct {
	th   *material.Theme
	mu   sync.Mutex
	text string
	kind Kind
}

func New() *View { return &View{th: material.NewTheme()} }

func (v *View) Set(text string, kind Kind) {
	v.mu.Lock()
	v.text, v.kind = text, kind
	v.mu.Unlock()
}

// Layout draws the line and returns the window size it needs, in pixels.
func (v *View) Layout(gtx layout.Context) image.Point {
	v.mu.Lock()
	text, kind := v.text, v.kind
	v.mu.Unlock()

	// Measure on an unconstrained single line, then paint the background
	// under it and replay the recorded text.
	m := op.Record(gtx.Ops)
	measure := gtx
	measure.Constraints = layout.Constraints{Max: image.Pt(1<<14, gtx.Constraints.Max.Y)}
	dims := layout.UniformInset(unit.Dp(8)).Layout(measure, func(gtx layout.Context) layout.Dimensions {
		l := material.Body1(v.th, text)
		l.MaxLines = 1
		l.Color = colors[kind]
		return l.Layout(gtx)
	})
	call := m.Stop()
	paint.FillShape(gtx.Ops, background, clip.Rect{Max: gtx.Constraints.Max}.Op())
	call.Add(gtx.Ops)
	return dims.Size
}
