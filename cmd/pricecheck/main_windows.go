//go:build windows

// Command pricecheck is the overlay (clipboard mode): press Ctrl+Alt+C (or
// Ctrl+C) on an item in the game and a price line appears at the cursor.
// The tray icon opens the settings (league, POESESSID), the log, and quits;
// running the .exe a second time quits too.
package main

import (
	"context"
	"errors"
	"fmt"
	"image"
	"log/slog"
	"math"
	"net/http"
	"os"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Mi7teR/poe-pricecheck/internal/config"
	"github.com/Mi7teR/poe-pricecheck/internal/platform/win"
	"github.com/Mi7teR/poe-pricecheck/internal/pricer"
	"github.com/Mi7teR/poe-pricecheck/internal/ui"
)

var version = "0.1.3"

const (
	className = "poe-pricecheck-listener"
	mutexName = `Local\poe-pricecheck`
	hideDist  = 200 // px the cursor may move away before the line hides
)

func main() {
	win.SetDPIAware()
	if first, _ := win.SingleInstance(mutexName); !first {
		win.AskRunningToQuit(className)
		return
	}

	dir, err := config.Dir()
	if err != nil {
		os.Exit(1)
	}
	logPath := setupLog(dir)
	cfg, cfgErr := config.Load(dir)
	slog.Info("start", "version", version, "dir", dir, "config_error", cfgErr, "poesessid_set", cfg.POESESSID != "")

	a := &overlay{view: ui.New(), cfg: cfg, cfgErr: cfgErr, logPath: logPath, dir: dir, items: make(chan string, 1)}
	a.win = new(app.Window)
	a.win.Option(
		app.Title("poe-pricecheck"),
		app.Decorated(false),
		app.TopMost(true),
		app.Size(unit.Dp(420), unit.Dp(36)),
	)
	go func() {
		if err := a.events(); err != nil {
			slog.Error("window", "err", err)
		}
		a.quit()
	}()
	app.Main()
}

type overlay struct {
	win     *app.Window
	view    *ui.View
	cfgErr  error
	logPath string
	dir     string
	items   chan string // latest copied item text; a newer copy replaces an unpriced one

	hwnd         uintptr
	listener     atomic.Pointer[win.Listener]
	pricer       atomic.Pointer[pricer.Pricer] // swapped when the settings change
	settingsOpen atomic.Bool

	cfgMu sync.Mutex
	cfg   config.Config

	mu      sync.Mutex
	shownAt [2]int32  // cursor where the line was shown
	hideAt  time.Time // zero = hidden
	size    image.Point
}

func (a *overlay) config() config.Config {
	a.cfgMu.Lock()
	defer a.cfgMu.Unlock()
	return a.cfg
}

func (a *overlay) options() pricer.Options {
	cfg := a.config()
	return pricer.Options{
		HTTP:      httpClient,
		UserAgent: "poe-pricecheck/" + version + " (github.com/Mi7teR/poe-pricecheck)",
		CacheDir:  filepath.Join(a.dir, "cache"),
		POESESSID: cfg.POESESSID,
		Config:    cfg,
	}
}

// One client for the whole run: its keep-alive connection is what makes
// searches skip the TLS handshake.
var httpClient = &http.Client{Timeout: 15 * time.Second}

// quit takes the tray icon down and exits.
func (a *overlay) quit() {
	a.listener.Load().RemoveTray()
	os.Exit(0)
}

// events runs Gio's loop. Platform calls that send messages to the window
// thread run on their own goroutines: the window thread waits for this loop
// while a frame is being drawn.
func (a *overlay) events() error {
	var ops op.Ops
	for {
		switch e := a.win.Event().(type) {
		case app.Win32ViewEvent:
			if e.HWND == 0 || a.hwnd != 0 {
				continue
			}
			a.hwnd = e.HWND
			// Gio's window thread waits for this loop until the event is
			// handled, so calls that message the window must run on that
			// thread: Run does that, before Gio activates the window.
			a.win.Run(func() {
				win.MakeOverlay(a.hwnd)
				win.Hide(a.hwnd) // from here on only our calls show it
			})
			slog.Info("window ready")
			go a.run()
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			need := a.view.Layout(gtx)
			e.Frame(gtx.Ops)
			if need != e.Size && need != a.size {
				a.size = need
				go win.Resize(a.hwnd, int32(need.X), int32(need.Y))
			}
		case app.DestroyEvent:
			return e.Err
		}
	}
}

func (a *overlay) run() {
	l, err := win.Listen(className, win.Events{
		Clipboard: a.onClipboard,
		Escape:    a.hide,
		Close:     func() { slog.Info("stop: asked by a second launch"); os.Exit(0) },
		TrayTip:   "poe-pricecheck " + version,
		TrayClick: a.openSettings,
		TrayMenu: []win.MenuItem{
			{Label: "Настройки…", Do: a.openSettings},
			{Label: "Открыть лог", Do: func() { win.OpenFile(a.logPath) }},
			{Label: "Выход", Do: func() { slog.Info("stop: tray"); a.quit() }},
		},
	})
	if err != nil {
		a.show("не удалось слушать буфер: "+err.Error(), ui.Error, 15*time.Second)
		slog.Error("listener", "err", err)
		return
	}
	a.listener.Store(l)
	slog.Info("listening")
	go a.watch()

	a.show("poe-pricecheck "+version+" · загрузка цен…", ui.Pending, time.Minute)
	for !a.load() {
		a.show("нет связи с трейдом или poe.ninja, повтор через 30 с", ui.Error, 30*time.Second)
		time.Sleep(30 * time.Second)
	}
	p := a.pricer.Load()
	hello := fmt.Sprintf("poe-pricecheck %s · лига %s · Ctrl+Alt+C на предмете · настройки в трее", version, p.League)
	if a.cfgErr != nil {
		hello = "config.toml с ошибкой, взяты значения по умолчанию · лог: " + a.logPath
	}
	a.show(hello, ui.Info, 8*time.Second)

	go a.background()
	for text := range a.items {
		a.price(text)
	}
}

// load builds a pricer from the current settings and swaps it in.
func (a *overlay) load() bool {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	p, err := pricer.Load(ctx, a.options())
	if err != nil {
		slog.Error("load", "err", err)
		return false
	}
	a.pricer.Store(p)
	slog.Info("ready", "league", p.League, "poesessid_set", p.Trade.POESESSID != "")
	return true
}

// onClipboard runs on the listener thread: hand the text over and return.
func (a *overlay) onClipboard(text string) {
	if !strings.HasPrefix(text, "Item Class: ") {
		return
	}
	for {
		select {
		case a.items <- text:
			return
		default: // an older copy is waiting: drop it, this one is newer
			select {
			case <-a.items:
			default:
			}
		}
	}
}

func (a *overlay) price(text string) {
	p := a.pricer.Load()
	if p == nil {
		return
	}
	hideAfter := a.config().HideAfter
	a.show("оцениваю…", ui.Pending, 20*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	t0 := time.Now()
	r, err := p.Price(ctx, text)
	took := time.Since(t0)
	var u *pricer.Unsupported
	switch {
	case errors.As(err, &u):
		a.show(u.Error(), ui.Info, hideAfter)
		slog.Info("unsupported", "what", u.What, "text", text)
	case err != nil:
		a.show("ошибка: "+err.Error(), ui.Error, hideAfter+3*time.Second)
		slog.Error("price", "err", err, "took", took, "text", text)
	default:
		line := r.Line(p.Divine())
		a.show(line, ui.Price, hideAfter)
		attrs := []any{"item", r.Item, "line", line, "source", r.Source, "took", took, "total", r.Total,
			"kept", len(r.Kept), "dropped", len(r.Dropped), "link", r.Link}
		for _, t := range r.Timings {
			attrs = append(attrs, "t_"+t.Step, t.Took)
		}
		slog.Info("price", append(attrs, "text", text)...)
	}
}

// show puts text at the cursor for d without taking focus from the game.
// While the line is up it stays where it first appeared.
func (a *overlay) show(text string, kind ui.Kind, d time.Duration) {
	a.view.Set(text, kind)
	a.win.Invalidate()
	before, title := win.Foreground()
	x, y := win.CursorPos()
	a.mu.Lock()
	wasHidden := a.hideAt.IsZero()
	if wasHidden {
		a.shownAt = [2]int32{x, y}
	}
	a.hideAt = time.Now().Add(d)
	at := a.shownAt
	a.mu.Unlock()
	win.ShowAt(a.hwnd, at[0], at[1])
	if l := a.listener.Load(); l != nil && wasHidden {
		l.GrabEscape(true)
	}
	if after, _ := win.Foreground(); after != before {
		slog.Warn("FOCUS STOLEN", "was", title)
	}
}

func (a *overlay) hide() {
	a.mu.Lock()
	shown := !a.hideAt.IsZero()
	a.hideAt = time.Time{}
	a.mu.Unlock()
	if shown {
		win.Hide(a.hwnd)
		if l := a.listener.Load(); l != nil {
			l.GrabEscape(false)
		}
	}
}

// watch hides the line on timeout or when the cursor moves away.
func (a *overlay) watch() {
	for range time.Tick(100 * time.Millisecond) {
		a.mu.Lock()
		hideAt, at := a.hideAt, a.shownAt
		a.mu.Unlock()
		if hideAt.IsZero() {
			continue
		}
		x, y := win.CursorPos()
		if time.Now().After(hideAt) || math.Hypot(float64(x-at[0]), float64(y-at[1])) > hideDist {
			a.hide()
		}
	}
}

// background refreshes poe.ninja prices and keeps the trade connection warm.
func (a *overlay) background() {
	refresh := time.NewTicker(20 * time.Minute)
	warm := time.NewTicker(45 * time.Second)
	for {
		select {
		case <-refresh.C:
			ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
			if err := a.pricer.Load().RefreshNinja(ctx, a.options()); err != nil {
				slog.Warn("ninja refresh", "err", err)
			}
			cancel()
		case <-warm.C:
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			_ = pricer.Warm(ctx, a.options())
			cancel()
		}
	}
}

// setupLog writes to pricecheck.log next to the config, moving a log over
// 5 MB to pricecheck.log.1 first.
func setupLog(dir string) string {
	path := filepath.Join(dir, "pricecheck.log")
	if st, err := os.Stat(path); err == nil && st.Size() > 5<<20 {
		_ = os.Rename(path, path+".1")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return ""
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(f, nil)))
	_ = debug.SetCrashOutput(f, debug.CrashOptions{}) // panics land in the log, not in a missing console
	return path
}
