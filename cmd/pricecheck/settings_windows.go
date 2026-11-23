//go:build windows

package main

import (
	"log/slog"
	"time"

	"github.com/Mi7teR/poe-pricecheck/internal/i18n"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Mi7teR/poe-pricecheck/internal/config"
	"github.com/Mi7teR/poe-pricecheck/internal/pricer"
	"github.com/Mi7teR/poe-pricecheck/internal/ui"
)

// openSettings shows the settings window; a second call while it is open
// does nothing. It is an ordinary window: it takes focus, unlike the line.
func (a *overlay) openSettings() {
	p := a.pricer.Load()
	if p == nil {
		a.show(i18n.T("settings open once prices are loaded"), ui.Info, 3*time.Second)
		return
	}
	if !a.settingsOpen.CompareAndSwap(false, true) {
		return
	}
	defer a.settingsOpen.Store(false)

	cfg := a.config()
	form := ui.NewSettings(p.Leagues, pricer.AutoLeague(p.Leagues), cfg.League, cfg.POESESSID)
	w := new(app.Window)
	w.Option(app.Title(i18n.T("poe-pricecheck — settings")), app.Size(unit.Dp(460), unit.Dp(640)))
	var ops op.Ops
	for {
		switch e := w.Event().(type) {
		case app.FrameEvent:
			gtx := app.NewContext(&ops, e)
			act := form.Layout(gtx)
			e.Frame(gtx.Ops)
			switch act {
			case ui.Saved:
				league, sess := form.Values()
				go a.applySettings(league, sess)
				w.Perform(system.ActionClose)
			case ui.Cancelled:
				w.Perform(system.ActionClose)
			}
		case app.DestroyEvent:
			return
		}
	}
}

// applySettings saves the league and POESESSID and reloads prices with them.
func (a *overlay) applySettings(league, sess string) {
	a.cfgMu.Lock()
	changed := a.cfg.League != league || a.cfg.POESESSID != sess
	a.cfg.League, a.cfg.POESESSID = league, sess
	cfg := a.cfg
	a.cfgMu.Unlock()
	if !changed {
		return
	}
	if err := config.Save(a.dir, cfg); err != nil {
		slog.Error("settings save", "err", err)
		a.show(i18n.Tf("cannot save config.toml: %s", err), ui.Error, 8*time.Second)
		return
	}
	slog.Info("settings saved", "league", league, "poesessid_set", sess != "") // never the session itself
	a.show(i18n.T("settings saved · loading prices…"), ui.Pending, time.Minute)
	if !a.load() {
		a.show(i18n.T("prices did not load, the old ones stay; see the log"), ui.Error, 8*time.Second)
		return
	}
	a.show(i18n.Tf("done · league %s", a.pricer.Load().League), ui.Info, 4*time.Second)
}
