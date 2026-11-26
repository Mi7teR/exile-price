package ui

import (
	"image"
	"testing"
	"time"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func cardsSize(c *Cards) image.Point {
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1.5, PxPerSp: 1.5},
		Constraints: layout.Exact(image.Pt(570, 600)),
	}
	size, _ := c.Layout(gtx)
	return size
}

func TestCardsStack(t *testing.T) {
	now := time.Date(2026, 11, 26, 13, 2, 7, 0, time.UTC)
	c := NewCards("новый лот")
	if s := cardsSize(c); s.Y != 0 {
		t.Errorf("empty stack is %v tall", s)
	}
	first := c.Add(1, "Tabula Rasa Simple Robe", nil, now)
	one := cardsSize(c)
	if one.X != 570 || one.Y <= 0 {
		t.Fatalf("one card: %v, want 570 wide", one)
	}
	c.Add(2, "wand", []string{"a", "b", "c", "d", "e"}, now)
	two := cardsSize(c)
	if two.Y <= 2*one.Y {
		t.Errorf("a card with filters is not taller: %v after %v", two, one)
	}

	again := c.Add(1, "Tabula Rasa Simple Robe", nil, now)
	if c.Len() != 2 || c.list[0].tab != 1 || c.list[0].count != 2 {
		t.Errorf("same tab again: len %d, top tab %d ×%d", c.Len(), c.list[0].tab, c.list[0].count)
	}
	if left := c.Remove(first); left != 2 {
		t.Errorf("the replaced card's timer removed something: %d left", left)
	}
	if left := c.Remove(again); left != 1 {
		t.Errorf("%d left, want 1", left)
	}
	for i := 10; i < 20; i++ {
		c.Add(i, "x", nil, now)
	}
	if c.Len() != maxCards {
		t.Errorf("%d cards, want at most %d", c.Len(), maxCards)
	}
}
