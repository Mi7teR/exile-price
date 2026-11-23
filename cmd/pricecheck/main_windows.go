//go:build windows

// Command pricecheck is the overlay (clipboard mode): press Ctrl+Alt+C (or
// Ctrl+C) on an item in the game and a price line appears at the cursor.
// The tray icon opens the settings (league, POESESSID), the log, and quits;
// running the .exe a second time quits too.
package main

import (
	"context"
	"errors"
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

	"github.com/Mi7teR/poe-pricecheck/internal/i18n"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Mi7teR/poe-pricecheck/internal/config"
	"github.com/Mi7teR/poe-pricecheck/internal/platform/win"
	"github.com/Mi7teR/poe-pricecheck/internal/pricer"
	"github.com/Mi7teR/poe-pricecheck/internal/trade"
	"github.com/Mi7teR/poe-pricecheck/internal/ui"
)

var version = "0.1.11"

const (
	className = "poe-pricecheck-listener"
	mutexName = `Local\poe-pricecheck`
	hideDist  = 200 // px the cursor may move away before the line hides
)

func main() {
	i18n.Set(i18n.FromSystem())
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

	before, _ := win.Foreground() // usually Explorer, where the .exe was started
	a := &overlay{before: before, view: ui.New(), cfg: cfg, cfgErr: cfgErr, logPath: logPath, dir: dir, items: make(chan string, 1)}
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
	before       uintptr // foreground window at start, given back after Gio takes it
	listener     atomic.Pointer[win.Listener]
	pricer       atomic.Pointer[pricer.Pricer] // swapped when the settings change
	settingsOpen atomic.Bool

	link atomic.Pointer[string] // the last trade search, for Ctrl+Alt+O

	last struct { // the last priced item; a repeat within a minute reuses it
		p    *pricer.Pricer
		text string
		line string
		at   time.Time
	}

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
			go win.GiveBackFocus(a.hwnd, a.before, a.win.Run) // Gio activates the window right after this event
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
		Open:      a.openSearch,
		Close:     func() { slog.Info("stop: asked by a second launch"); os.Exit(0) },
		TrayTip:   "poe-pricecheck " + version,
		TrayClick: a.openSettings,
		TrayMenu: []win.MenuItem{
			{Label: i18n.T("Open the last search (Ctrl+Alt+O)"), Do: a.openSearch},
			{Label: i18n.T("Settings…"), Do: a.openSettings},
			{Label: i18n.T("Open log"), Do: func() { win.OpenFile(a.logPath) }},
			{Label: i18n.T("Quit"), Do: func() { slog.Info("stop: tray"); a.quit() }},
		},
	})
	if err != nil {
		a.show(i18n.Tf("cannot listen to the clipboard: %s", err), ui.Error, 15*time.Second)
		slog.Error("listener", "err", err)
		return
	}
	a.listener.Store(l)
	slog.Info("listening")
	go a.watch()

	a.show(i18n.Tf("poe-pricecheck %s · loading prices…", version), ui.Pending, time.Minute)
	for !a.load() {
		a.show(i18n.T("no connection to the trade site or poe.ninja, retrying in 30 s"), ui.Error, 30*time.Second)
		time.Sleep(30 * time.Second)
	}
	p := a.pricer.Load()
	hello := i18n.Tf("poe-pricecheck %s · league %s · Ctrl+Alt+C on an item · settings in the tray", version, p.League)
	if a.cfgErr != nil {
		hello = i18n.Tf("config.toml has an error, using defaults · log: %s", a.logPath)
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
	if a.last.p == p && a.last.text == text && time.Since(a.last.at) < time.Minute {
		a.show(a.last.line, ui.Price, hideAfter) // a second press must not spend trade searches
		slog.Info("price cached", "line", a.last.line)
		return
	}
	a.show(i18n.T("pricing…"), ui.Pending, 20*time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	t0 := time.Now()
	r, err := p.Price(ctx, text)
	took := time.Since(t0)
	var u *pricer.Unsupported
	var rl *trade.RateLimitError
	switch {
	case errors.As(err, &u):
		a.show(u.Error(), ui.Info, hideAfter)
		slog.Info("unsupported", "what", u.What, "text", text)
	case errors.As(err, &rl):
		a.show(i18n.Tf("trade request limit · try again in %d s", int(rl.Wait.Seconds()+0.999)), ui.Error, hideAfter)
		slog.Warn("rate limit", "policy", rl.Policy, "wait", rl.Wait, "took", took)
	case err != nil:
		a.show(i18n.Tf("error: %s", err), ui.Error, hideAfter+3*time.Second)
		slog.Error("price", "err", err, "took", took, "text", text)
	default:
		line := r.Line(p.Divine())
		a.show(line, ui.Price, hideAfter)
		a.last.p, a.last.text, a.last.line, a.last.at = p, text, line, time.Now()
		if r.Link != "" {
			a.link.Store(&r.Link)
		}
		attrs := []any{"item", r.Item, "line", line, "source", r.Source, "took", took, "total", r.Total,
			"kept", len(r.Kept), "dropped", len(r.Dropped), "link", r.Link}
		for _, t := range r.Timings {
			attrs = append(attrs, "t_"+t.Step, t.Took)
		}
		slog.Info("price", append(attrs, "text", text)...)
	}
}

// openSearch opens the last trade search in the browser, to see the
// listings behind the line.
func (a *overlay) openSearch() {
	l := a.link.Load()
	if l == nil {
		a.show(i18n.T("no trade search yet"), ui.Info, 3*time.Second)
		return
	}
	win.OpenFile(*l)
	slog.Info("search opened")
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
