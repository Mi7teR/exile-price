package ui

import (
	"fmt"
	"image"
	"image/color"
	"slices"
	"sync"
	"time"

	"gioui.org/gesture"
	"gioui.org/io/pointer"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"
)

const (
	cardWidth = unit.Dp(380)
	cardLines = 3 // filter lines on a card before "+N"
	maxCards  = 5 // older cards fall off the bottom
)

var (
	cardBackground = color.NRGBA{R: 24, G: 28, B: 26, A: 250}
	cardSeparator  = color.NRGBA{R: 60, G: 70, B: 64, A: 255}
	cardHead       = color.NRGBA{R: 150, G: 160, B: 155, A: 255}
	cardLine       = color.NRGBA{R: 205, G: 210, B: 205, A: 255}
)

type card struct {
	id    int64
	tab   int
	count int // listings from this tab while its card is up
	at    time.Time
	title string
	lines []string
	click gesture.Click
}

// Cards is a stack of live search notices, newest on top. A new listing from
// a tab that already has a card moves that card up and counts it. Methods
// may be called from any goroutine.
type Cards struct {
	th   *material.Theme
	head string // "new listing", in the interface language
	mu   sync.Mutex
	next int64
	list []*card
}

func NewCards(head string) *Cards { return &Cards{th: material.NewTheme(), head: head} }

// Add puts a card on top and returns its id for Remove.
func (c *Cards) Add(tab int, title string, lines []string, now time.Time) int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.next++
	k := &card{id: c.next, tab: tab, count: 1, at: now, title: title, lines: lines}
	if i := slices.IndexFunc(c.list, func(o *card) bool { return o.tab == tab }); i >= 0 {
		k.count += c.list[i].count
		c.list = slices.Delete(c.list, i, i+1)
	}
	c.list = append([]*card{k}, c.list...)
	if len(c.list) > maxCards {
		c.list = c.list[:maxCards]
	}
	return k.id
}

// Remove drops the card with this id, if it is still the latest one for its
// tab, and returns how many cards are left.
func (c *Cards) Remove(id int64) int {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.list = slices.DeleteFunc(c.list, func(k *card) bool { return k.id == id })
	return len(c.list)
}

func (c *Cards) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.list)
}

// Layout draws the stack and returns the window size it needs, in pixels,
// and the tab of a card clicked since the last frame (0 if none). A clicked
// card is removed.
func (c *Cards) Layout(gtx layout.Context) (image.Point, int) {
	c.mu.Lock()
	list := slices.Clone(c.list)
	c.mu.Unlock()
	clicked := 0
	for _, k := range list {
		for {
			ev, ok := k.click.Update(gtx.Source)
			if !ok {
				break
			}
			if ev.Kind == gesture.KindClick {
				clicked = k.tab
				c.Remove(k.id)
			}
		}
	}
	if clicked != 0 {
		c.mu.Lock()
		list = slices.Clone(c.list)
		c.mu.Unlock()
	}

	w := gtx.Dp(cardWidth)
	paint.FillShape(gtx.Ops, cardSeparator, clip.Rect{Max: gtx.Constraints.Max}.Op())
	y := 0
	for i, k := range list {
		if i > 0 {
			y++ // a 1px separator shows through
		}
		off := op.Offset(image.Pt(0, y)).Push(gtx.Ops)
		y += c.card(gtx, k, w)
		off.Pop()
	}
	return image.Pt(w, y), clicked
}

// card draws one card at the origin and returns its height.
func (c *Cards) card(gtx layout.Context, k *card, w int) int {
	label := func(l material.LabelStyle, col color.NRGBA) layout.FlexChild {
		l.MaxLines = 1
		l.Color = col
		return layout.Rigid(l.Layout)
	}
	head := c.head
	if k.count > 1 {
		head += fmt.Sprintf(" ×%d", k.count)
	}
	rows := []layout.FlexChild{
		label(material.Caption(c.th, head+" · "+k.at.Format("15:04:05")), cardHead),
		label(material.Subtitle1(c.th, k.title), colors[Notice]),
	}
	lines := k.lines
	if len(lines) > cardLines {
		lines = append(lines[:cardLines:cardLines], fmt.Sprintf("+%d", len(k.lines)-cardLines))
	}
	for _, s := range lines {
		rows = append(rows, label(material.Body2(c.th, s), cardLine))
	}

	cg := gtx
	cg.Constraints = layout.Constraints{Min: image.Pt(w, 0), Max: image.Pt(w, 1<<14)}
	m := op.Record(gtx.Ops)
	dims := layout.Inset{Top: unit.Dp(8), Bottom: unit.Dp(8), Left: unit.Dp(14), Right: unit.Dp(10)}.Layout(cg,
		func(gtx layout.Context) layout.Dimensions {
			return layout.Flex{Axis: layout.Vertical}.Layout(gtx, rows...)
		})
	call := m.Stop()

	h := dims.Size.Y
	rect := clip.Rect{Max: image.Pt(w, h)}
	paint.FillShape(gtx.Ops, cardBackground, rect.Op())
	paint.FillShape(gtx.Ops, colors[Notice], clip.Rect{Max: image.Pt(gtx.Dp(4), h)}.Op())
	call.Add(gtx.Ops)
	area := rect.Push(gtx.Ops)
	k.click.Add(gtx.Ops)
	pointer.CursorPointer.Add(gtx.Ops)
	area.Pop()
	return h
}
