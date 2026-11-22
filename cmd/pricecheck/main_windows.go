//go:build windows

// Command pricecheck is the overlay. Spike build: every 5 s it shows a
// one-line window at the cursor for 2 s and logs whether the foreground
// window (the game) kept focus, and exits after 60 shows (5 min). Log: %APPDATA%\poe-pricecheck\spike.log.
package main

import (
	"fmt"
	"image/color"
	"log/slog"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/op/clip"
	"gioui.org/op/paint"
	"gioui.org/unit"
	"gioui.org/widget/material"

	"github.com/Mi7teR/poe-pricecheck/internal/platform/win"
)

var version = "0.0.1-spike"

func main() {
	logPath := setupLog()
	slog.Info("start", "version", version, "log", logPath)

	w := new(app.Window)
	w.Option(
		app.Title("poe-pricecheck"),
		app.Decorated(false),
		app.TopMost(true),
		app.Size(unit.Dp(360), unit.Dp(36)),
	)
	go func() {
		if err := loop(w); err != nil {
			slog.Error("window", "err", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()
	app.Main()
}

func loop(w *app.Window) error {
	th := material.NewTheme()
	var ops op.Ops
	var shows atomic.Int32
	started := false
	for {
		switch e := w.Event().(type) {
		case app.Win32ViewEvent:
			if e.HWND == 0 || started {
				continue
			}
			started = true
			win.MakeOverlay(e.HWND)
			win.Hide(e.HWND) // Gio activated it on create; from here on only our calls show it
			go cycle(w, e.HWND, &shows)
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			paint.FillShape(gtx.Ops, color.NRGBA{R: 20, G: 20, B: 24, A: 255}, clip.Rect{Max: gtx.Constraints.Max}.Op())
			layout.UniformInset(unit.Dp(8)).Layout(gtx, func(gtx layout.Context) layout.Dimensions {
				l := material.Body1(th, fmt.Sprintf("~6–9 div · 14 лотов, 3 фиксера  (spike #%d)", shows.Load()))
				l.Color = color.NRGBA{R: 230, G: 210, B: 150, A: 255}
				return l.Layout(gtx)
			})
			e.Frame(gtx.Ops)
		case app.DestroyEvent:
			return e.Err
		}
	}
}

// cycle shows the window at the cursor for 2 s every 5 s and records focus.
func cycle(w *app.Window, hwnd uintptr, shows *atomic.Int32) {
	for range 60 {
		time.Sleep(3 * time.Second)
		before, title := win.Foreground()
		x, y := win.CursorPos()
		n := shows.Add(1)
		w.Invalidate()
		win.ShowAt(hwnd, x+16, y+16)
		time.Sleep(50 * time.Millisecond)
		after, _ := win.Foreground()
		if after != before {
			slog.Warn("FOCUS STOLEN", "show", n, "was", title)
		} else {
			slog.Info("shown, focus kept", "show", n, "foreground", title)
		}
		time.Sleep(2 * time.Second)
		win.Hide(hwnd)
	}
	slog.Info("done")
	os.Exit(0)
}

func setupLog() string {
	dir, err := os.UserConfigDir() // %APPDATA%
	if err != nil {
		dir = "."
	}
	dir = filepath.Join(dir, "poe-pricecheck")
	_ = os.MkdirAll(dir, 0o755)
	path := filepath.Join(dir, "spike.log")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return ""
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, nil)))
	return path
}
