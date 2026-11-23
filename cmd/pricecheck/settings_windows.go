//go:build windows

package main

import (
	"log/slog"
	"time"

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
		a.show("настройки откроются после загрузки цен", ui.Info, 3*time.Second)
		return
	}
	if !a.settingsOpen.CompareAndSwap(false, true) {
		return
	}
	defer a.settingsOpen.Store(false)

	cfg := a.config()
	form := ui.NewSettings(p.Leagues, pricer.AutoLeague(p.Leagues), cfg.League, cfg.POESESSID)
	w := new(app.Window)
	w.Option(app.Title("poe-pricecheck — настройки"), app.Size(unit.Dp(460), unit.Dp(640)))
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
		a.show("не удалось сохранить config.toml: "+err.Error(), ui.Error, 8*time.Second)
		return
	}
	slog.Info("settings saved", "league", league, "poesessid_set", sess != "") // never the session itself
	a.show("настройки сохранены · загрузка цен…", ui.Pending, time.Minute)
	if !a.load() {
		a.show("цены не загрузились, остались прежние; подробности в логе", ui.Error, 8*time.Second)
		return
	}
	a.show("готово · лига "+a.pricer.Load().League, ui.Info, 4*time.Second)
}
