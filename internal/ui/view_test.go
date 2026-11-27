package ui

import (
	"image"
	"testing"

	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/unit"
)

func size(v *View, text string) image.Point {
	v.Set(text, Price)
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1.5, PxPerSp: 1.5}, // 150% scale
		Constraints: layout.Exact(image.Pt(420, 54)),
	}
	return v.Layout(gtx)
}

func TestLayoutSizesToText(t *testing.T) {
	v := New()
	short := size(v, "~4.0c · poe.ninja")
	long := size(v, "exile-price 0.1.0 · лига Covens · Ctrl+Alt+C на предмете · лог: C:\\Users\\tester\\AppData\\Roaming\\exile-price\\pricecheck.log")
	if short.X <= 0 || short.Y <= 0 {
		t.Fatalf("short = %v", short)
	}
	if long.X <= 420 {
		t.Errorf("long line measured %v: must ask for more than the 420px window, not wrap", long)
	}
	if long.Y != short.Y {
		t.Errorf("one line each: heights %d vs %d", short.Y, long.Y)
	}
}

func TestLayoutTwoLines(t *testing.T) {
	v := New()
	one := size(v, "~4.0c")
	v.SetSub([]Span{{Text: "instead of Ring 2: ", Kind: Muted}, {Text: "DPS -13.9%", Kind: Worse}, {Text: " · Life +49", Kind: Better}})
	two := size(v, "~4.0c")
	if two.Y <= one.Y {
		t.Errorf("second line must add height: %d vs %d", two.Y, one.Y)
	}
	if two.X <= one.X {
		t.Errorf("width must be the wider line: %d vs %d", two.X, one.X)
	}
	v.SetSub(nil)
	if back := size(v, "~4.0c"); back != one {
		t.Errorf("SetSub(nil): got %v, want %v", back, one)
	}
}

func TestLayoutPendingHasSpinner(t *testing.T) {
	v := New()
	plain := size(v, "ищу на трейде…")
	v.Set("ищу на трейде…", Pending)
	var ops op.Ops
	gtx := layout.Context{
		Ops:         &ops,
		Metric:      unit.Metric{PxPerDp: 1.5, PxPerSp: 1.5},
		Constraints: layout.Exact(image.Pt(420, 54)),
	}
	pending := v.Layout(gtx)
	if pending.X <= plain.X {
		t.Errorf("spinner must add width: %d vs %d", pending.X, plain.X)
	}
	if pending.Y != plain.Y {
		t.Errorf("spinner must not change the height: %d vs %d", pending.Y, plain.Y)
	}
}
