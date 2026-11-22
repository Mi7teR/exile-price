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
	long := size(v, "poe-pricecheck 0.1.0 · лига Covens · Ctrl+Alt+C на предмете · лог: C:\\Users\\tester\\AppData\\Roaming\\poe-pricecheck\\pricecheck.log")
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
