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
	"io/fs"
	"log/slog"
	"math"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Mi7teR/exile-calc/engine"

	"github.com/Mi7teR/exile-price/internal/i18n"
	"github.com/Mi7teR/exile-price/internal/logfile"

	"gioui.org/app"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Mi7teR/exile-price/internal/build"
	"github.com/Mi7teR/exile-price/internal/config"
	"github.com/Mi7teR/exile-price/internal/notify"
	"github.com/Mi7teR/exile-price/internal/platform/win"
	"github.com/Mi7teR/exile-price/internal/pricer"
	"github.com/Mi7teR/exile-price/internal/trade"
	"github.com/Mi7teR/exile-price/internal/ui"
)

var version = "0.1.17"

const (
	className = "exile-price-listener"
	mutexName = `Local\exile-price`
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
		app.Title("exile-price"),
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
	a.notice = &notice{cards: ui.NewCards(i18n.T("new listing")), win: new(app.Window)}
	a.notice.win.Option(
		app.Title("exile-price live"),
		app.Decorated(false),
		app.TopMost(true),
		app.Size(unit.Dp(380), unit.Dp(60)),
	)
	go func() {
		if err := a.notice.events(); err != nil {
			slog.Error("notice window", "err", err)
		}
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

	notice *notice // "new listing" in the corner, from the browser extension

	store     atomic.Pointer[build.Store] // nil until the engine is ready
	seq       atomic.Uint64               // one per line; a late build change for an older line is dropped
	issue     buildIssue                  // a build problem: shown once, logged once
	engineErr atomic.Pointer[error]       // the engine failed to start

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
	lastSub struct { // the build change of the last compared item, for the repeat
		text  string
		spans []ui.Span
	}
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
		UserAgent: "exile-price/" + version + " (github.com/Mi7teR/exile-price)",
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
		TrayTip:   "exile-price " + version,
		TrayClick: a.openSettings,
		TrayMenu: []win.MenuItem{
			{Label: i18n.T("Open the last search (Ctrl+Alt+O)"), Do: a.openSearch},
			{Label: i18n.T("Load build from clipboard"), Do: func() { go a.loadClipboardBuild() }},
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
	go a.notice.serve(a.statText)

	buildErr := make(chan error, 1)
	go func() { buildErr <- a.loadBuild() }() // alongside the prices

	a.show(i18n.Tf("exile-price %s · loading prices…", version), ui.Pending, time.Minute)
	for !a.load() {
		a.show(i18n.T("no connection to the trade site or poe.ninja, retrying in 30 s"), ui.Error, 30*time.Second)
		time.Sleep(30 * time.Second)
	}
	p := a.pricer.Load()
	hello := i18n.Tf("exile-price %s · league %s · Ctrl+Alt+C on an item · settings in the tray", version, p.League)
	if a.cfgErr != nil {
		hello = i18n.Tf("config.toml has an error, using defaults · log: %s", a.logPath)
	}
	a.show(hello, ui.Info, 8*time.Second)
	go func() {
		if <-buildErr == nil {
			return
		}
		seq := a.seq.Load()
		time.Sleep(8 * time.Second)
		if a.seq.Load() != seq {
			return // a newer line is up; compare puts the note under the next price
		}
		if note, gone := a.issue.take(); note != "" {
			a.show(note, ui.Error, 8*time.Second)
			a.issue.shown(gone)
		}
	}()

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
	n := a.newLine()
	gear := build.Equippable(text) // no build change for currency, gems, maps, jewels
	hideAfter := a.config().HideAfter
	if a.last.p == p && a.last.text == text && time.Since(a.last.at) < time.Minute {
		a.put(a.last.line, ui.Price, hideAfter) // a second press must not spend trade searches
		if gear {
			a.restoreSub(n, text) // until the compare below: the build may have been saved in PoB
			go a.compare(n, text)
		}
		slog.Info("price cached", "line", a.last.line)
		return
	}
	a.put(i18n.T("pricing…"), ui.Pending, 20*time.Second)
	if gear {
		go a.compare(n, text) // after show: the line is up when the change arrives
		// The line hides if the mouse moves away while pricing, and the
		// price brings it back: give it back its build change too.
		defer a.restoreSub(n, text)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	st := &stages{show: func(s string) { a.stage(n, s) }}
	ctx = trade.WithProgress(ctx, st.report)
	t0 := time.Now()
	r, err := p.Price(ctx, text)
	st.stop()
	took := time.Since(t0)
	var u *pricer.Unsupported
	var rl *trade.RateLimitError
	switch {
	case errors.As(err, &u):
		a.put(u.Error(), ui.Info, hideAfter)
		slog.Info("unsupported", "what", u.What, "text", text)
	case errors.As(err, &rl):
		a.put(i18n.Tf("trade request limit · try again in %d s", int(rl.Wait.Seconds()+0.999)), ui.Error, hideAfter)
		slog.Warn("rate limit", "policy", rl.Policy, "wait", rl.Wait, "took", took)
	case err != nil:
		a.put(i18n.Tf("error: %s", err), ui.Error, hideAfter+3*time.Second)
		slog.Error("price", "err", err, "took", took, "text", text)
	default:
		line := r.Line(p.Divine())
		a.put(line, ui.Price, hideAfter)
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

// loadBuild starts the engine and loads the build chosen in the config.
func (a *overlay) loadBuild() error {
	calc, err := engine.New()
	if err != nil {
		slog.Error("build engine", "err", err)
		a.engineErr.Store(&err)
		a.issue.set(engineErrText(err), false) // shown once: after the greeting or under the next price
		return err
	}
	s := build.NewStore(a.dir, calc)
	a.store.Store(s)
	cfg := a.config()
	b, err := s.Use(cfg.BuildSource, cfg.PoBBuild)
	logBuild(cfg.BuildSource, b, err)
	if err != nil {
		a.issue.set(buildErrText(err), errors.Is(err, fs.ErrNotExist)) // shown once: after the greeting or under the next price
	}
	return err
}

// logBuild logs a build load: class and level only, never the code.
func logBuild(source string, b *build.Build, err error) {
	switch {
	case err != nil:
		slog.Error("build load", "source", source, "err", err)
	case b == nil:
		slog.Info("build: none", "source", source)
	default:
		slog.Info("build loaded", "source", source, "class", b.Class, "level", b.Level)
	}
}

// compare puts the build change under the line of item n, unless a newer
// item came or the line was hidden meanwhile.
func (a *overlay) compare(n uint64, text string) {
	s := a.store.Load()
	if s == nil {
		a.putNote(n) // the engine failed to start
		return
	}
	b, err := s.Current()
	switch {
	case err == nil:
		a.issue.ok()
	case a.issue.failed(err):
		slog.Warn("build", "err", err) // once per broken state, not per press
	}
	a.putNote(n)
	if b == nil {
		return
	}
	t0 := time.Now()
	d, err := b.Compare(text)
	switch {
	case errors.Is(err, build.ErrNoSlot):
		slog.Info("build: no slot")
		return
	case err != nil:
		slog.Warn("build compare", "err", err)
		return
	}
	shown := a.putSub(n, text, subSpans(d.Segments()))
	slog.Info("build change", "line", d.String(), "took", time.Since(t0), "shown", shown, "missing", d.Missing)
}

// putNote puts a build problem not shown yet under the line of item n;
// if that line is gone it waits for the next one.
func (a *overlay) putNote(n uint64) {
	note, gone := a.issue.take()
	if note == "" {
		return
	}
	if a.putSub(n, "", []ui.Span{{Text: note, Kind: ui.Worse}}) {
		a.issue.shown(gone)
	} else {
		a.issue.putBack(note, gone) // not seen: try again under the next line
	}
}

// restoreSub puts the kept build change of text under the line of item n.
func (a *overlay) restoreSub(n uint64, text string) {
	a.mu.Lock()
	kept := a.lastSub
	a.mu.Unlock()
	if kept.text == text {
		a.putSub(n, "", kept.spans)
	}
}

// putSub sets the second line if item n is still the current one and its
// line is up; a hidden line is not shown again. A non-empty text also
// keeps the spans for a repeat of that item.
func (a *overlay) putSub(n uint64, text string, spans []ui.Span) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.seq.Load() != n {
		return false
	}
	if text != "" {
		a.lastSub.text, a.lastSub.spans = text, spans
	}
	if a.hideAt.IsZero() {
		return false
	}
	a.view.SetSub(spans)
	a.win.Invalidate()
	return true
}

// loadClipboardBuild loads a PoB code from the clipboard (tray menu) and
// makes the clipboard the build source.
func (a *overlay) loadClipboardBuild() {
	s := a.store.Load()
	if s == nil {
		a.engineNotReady()
		return
	}
	text, err := win.ReadClipboard()
	if err != nil {
		slog.Warn("build clipboard", "err", err)
		a.show(i18n.Tf("cannot read the clipboard: %s", err), ui.Error, 6*time.Second)
		return
	}
	b, err := s.SaveClipboard(text)
	if err != nil {
		slog.Warn("build clipboard", "len", len(text), "err", err)
		a.show(clipboardErrText(text, err), ui.Error, 8*time.Second)
		return
	}
	slog.Info("build loaded", "source", config.SourceClipboard, "class", b.Class, "level", b.Level, "len", len(text))
	a.forgetSub()
	a.issue.ok()
	a.cfgMu.Lock()
	a.cfg.BuildSource = config.SourceClipboard
	err = config.Save(a.dir, a.cfg) // under cfgMu: a later save never writes an older config
	a.cfgMu.Unlock()
	if err != nil {
		slog.Error("settings save", "err", err)
		a.show(i18n.Tf("cannot save config.toml: %s", err), ui.Error, 8*time.Second)
		return
	}
	a.show(i18n.Tf("build loaded: %s", b.Summary()), ui.Info, 6*time.Second)
}

// engineNotReady says the build engine is still starting or failed to.
func (a *overlay) engineNotReady() {
	msg := i18n.T("the engine is still loading")
	if err := a.engineErr.Load(); err != nil {
		msg = engineErrText(*err)
	}
	slog.Warn("build: engine not ready", "failed", a.engineErr.Load() != nil)
	a.show(msg, ui.Error, 6*time.Second)
}

// forgetSub drops the kept build change and any compare still running: the
// build is another one now.
func (a *overlay) forgetSub() {
	a.mu.Lock()
	a.lastSub.text, a.lastSub.spans = "", nil
	a.mu.Unlock()
	a.newLine()
}

// newLine starts a line without a second line: a compare still running for
// the previous one is dropped. It returns the new line's number.
func (a *overlay) newLine() uint64 {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.view.SetSub(nil)
	return a.seq.Add(1)
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

// stage updates the pending line of item n with the step it waits for. A
// hidden line stays hidden: the price itself brings it back.
func (a *overlay) stage(n uint64, text string) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.seq.Load() != n || a.hideAt.IsZero() {
		return
	}
	a.view.Set(text, ui.Pending)
	a.win.Invalidate()
}

// show puts a message at the cursor for d without taking focus from the
// game, with no second line. While the line is up it stays where it first
// appeared.
func (a *overlay) show(text string, kind ui.Kind, d time.Duration) {
	a.newLine()
	a.put(text, kind, d)
}

// put is show for the lines of a price: the build change under them stays.
func (a *overlay) put(text string, kind ui.Kind, d time.Duration) {
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
	a.view.SetSub(nil)
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

// setupLog writes to pricecheck.log next to the config; with the older
// pricecheck.log.1 it stays under 10 MB.
func setupLog(dir string) string {
	path := filepath.Join(dir, "pricecheck.log")
	l, err := logfile.Open(path, 10<<20, func(f *os.File) {
		_ = debug.SetCrashOutput(f, debug.CrashOptions{}) // panics land in the log, not in a missing console
	})
	if err != nil {
		return ""
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(l, nil)))
	return path
}

// notice is the second overlay window: a stack of "new listing" cards in
// the top right corner when a trade live search tab finds something (see
// extension/). A click on a card brings that browser tab to the front.
type notice struct {
	win   *app.Window
	cards *ui.Cards
	hwnd  atomic.Uintptr
	size  image.Point

	mu       sync.Mutex
	browsers map[int]string // tab → the browser's exe, so a click opens the same browser
}

// noticeTTL is how long a card stays up.
const noticeTTL = 20 * time.Second

func (n *notice) events() error {
	var ops op.Ops
	for {
		switch e := n.win.Event().(type) {
		case app.Win32ViewEvent:
			if e.HWND == 0 || n.hwnd.Load() != 0 {
				continue
			}
			n.hwnd.Store(e.HWND)
			before, _ := win.Foreground()
			n.win.Run(func() {
				win.MakeOverlay(e.HWND)
				win.Hide(e.HWND)
			})
			go win.GiveBackFocus(e.HWND, before, n.win.Run)
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			need, tab := n.cards.Layout(gtx)
			e.Frame(gtx.Ops)
			h := n.hwnd.Load()
			if tab != 0 {
				go n.open(tab)
			}
			if need.Y > 0 && need != e.Size && need != n.size {
				n.size = need
				go func() {
					win.Resize(h, int32(need.X), int32(need.Y))
					win.ShowTopRight(h) // keep the right edge in the corner
				}()
			}
		case app.DestroyEvent:
			return e.Err
		}
	}
}

// serve listens on 127.0.0.1 for the extension. A busy port only costs the
// notices: the price check keeps working.
func (n *notice) serve(stat notify.StatText) {
	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", notify.Port))
	if err != nil {
		slog.Warn("live notices off", "err", err)
		return
	}
	mux := http.NewServeMux()
	mux.Handle("/notify", notify.Handler(notify.ExtensionID, stat, n.show))
	mux.HandleFunc("/focus", notify.FocusPage)
	slog.Info("live notices on", "port", notify.Port)
	if err := http.Serve(ln, mux); err != nil {
		slog.Warn("live notices stopped", "err", err)
	}
}

func (n *notice) show(s notify.Search, tab int, from uint16) {
	h := n.hwnd.Load()
	if h == 0 {
		return
	}
	exe, err := win.TCPClientExe(from, notify.Port)
	slog.Info("live notice", "search", s.String(), "browser", exe, "err", err)
	n.mu.Lock()
	if n.browsers == nil {
		n.browsers = map[int]string{}
	}
	n.browsers[tab] = exe
	n.mu.Unlock()
	id := n.cards.Add(tab, s.Name, s.Filters, time.Now())
	n.win.Invalidate()
	win.ShowTopRight(h)
	win.Beep()
	time.AfterFunc(noticeTTL, func() {
		if n.cards.Remove(id) == 0 {
			win.Hide(h)
		}
		n.win.Invalidate()
	})
}

// statText names the trade stats in a notice; before prices load there are
// none and the notice shows only the search's name.
func (a *overlay) statText(id string) (string, bool) {
	p := a.pricer.Load()
	if p == nil || p.Stats == nil {
		return "", false
	}
	return p.Stats.Text(id)
}

// open has the extension bring the clicked card's tab to the front, and
// hides the window if that was the last card.
func (n *notice) open(tab int) {
	if n.cards.Len() == 0 {
		win.Hide(n.hwnd.Load())
	}
	n.mu.Lock()
	exe := n.browsers[tab]
	n.mu.Unlock()
	url := notify.FocusURL(tab)
	if exe != "" {
		if err := exec.Command(exe, url).Start(); err == nil {
			slog.Info("live notice opened", "browser", exe)
			return
		}
	}
	win.OpenFile(url) // the default browser
	slog.Info("live notice opened")
}
