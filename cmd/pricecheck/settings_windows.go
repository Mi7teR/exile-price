//go:build windows

package main

import (
	"errors"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"github.com/Mi7teR/exile-price/internal/i18n"

	"gioui.org/app"
	"gioui.org/io/system"
	"gioui.org/op"
	"gioui.org/unit"

	"github.com/Mi7teR/exile-price/internal/build"
	"github.com/Mi7teR/exile-price/internal/config"
	"github.com/Mi7teR/exile-price/internal/pricer"
	"github.com/Mi7teR/exile-price/internal/ui"
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
	dir := cfg.PoBDir
	if dir == "" {
		dir = build.DefaultPoBDir()
	}
	_, err := os.Stat(filepath.Join(a.dir, "build.txt"))
	form := ui.NewSettings(p.Leagues, pricer.AutoLeague(p.Leagues), cfg.League, cfg.POESESSID).
		WithBuilds(ui.BuildChoice{Dir: dir, Source: cfg.BuildSource, Path: cfg.PoBBuild}, err == nil, buildEntries).
		WithLanguage(cfg.Language)
	w := new(app.Window)
	w.Option(app.Title(i18n.T("exile-price — settings")), app.Size(unit.Dp(460), unit.Dp(760)))
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
				go a.applySettings(form.Language(), league, sess, form.Build())
				w.Perform(system.ActionClose)
			case ui.Cancelled:
				w.Perform(system.ActionClose)
			}
		case app.DestroyEvent:
			return
		}
	}
}

// applySettings saves the language, league, POESESSID and build choice.
// The language applies at once; prices are reloaded only when the league or
// POESESSID changed; a new build is just loaded.
func (a *overlay) applySettings(lang, league, sess string, c ui.BuildChoice) {
	def := build.DefaultPoBDir()
	a.cfgMu.Lock()
	old := a.cfg
	a.cfg = withBuild(a.cfg, c, def)
	a.cfg.Language, a.cfg.League, a.cfg.POESESSID = lang, league, sess
	cfg := a.cfg
	prices := old.League != league || old.POESESSID != sess
	newBuild := old.BuildSource != cfg.BuildSource || old.PoBBuild != cfg.PoBBuild
	newLang := i18n.Parse(old.Language) != i18n.Parse(lang)
	var err error
	if prices || newBuild || newLang || old.PoBDir != cfg.PoBDir {
		err = config.Save(a.dir, cfg) // under cfgMu: a later save never writes an older config
	}
	a.cfgMu.Unlock()
	if !prices && !newBuild && !newLang && old.PoBDir == cfg.PoBDir {
		return
	}
	if newLang {
		i18n.Set(i18n.Parse(lang))
	}
	if err != nil {
		slog.Error("settings save", "err", err)
		a.show(i18n.Tf("cannot save config.toml: %s", err), ui.Error, 8*time.Second)
		return
	}
	slog.Info("settings saved", "language", lang, "league", league, "poesessid_set", sess != "", // never the session itself
		"build_source", cfg.BuildSource, "pob_build", cfg.PoBBuild, "pob_dir", cfg.PoBDir)
	if newBuild {
		a.useBuild(cfg)
	}
	if !prices {
		if newLang && !newBuild {
			a.show(i18n.T("settings saved"), ui.Info, 3*time.Second)
		}
		return
	}
	a.show(i18n.T("settings saved · loading prices…"), ui.Pending, time.Minute)
	if !a.load() {
		a.show(i18n.T("prices did not load, the old ones stay; see the log"), ui.Error, 8*time.Second)
		return
	}
	a.show(i18n.Tf("done · league %s", a.pricer.Load().League), ui.Info, 4*time.Second)
}

// useBuild switches to the build chosen in the settings and says so.
func (a *overlay) useBuild(cfg config.Config) {
	s := a.store.Load()
	if s == nil {
		a.engineNotReady() // once it starts, it loads the config's build itself
		return
	}
	a.forgetSub()
	a.issue.ok() // the line below says how the new build went
	b, err := s.Use(cfg.BuildSource, cfg.PoBBuild)
	logBuild(cfg.BuildSource, b, err)
	if err != nil {
		a.issue.shown(errors.Is(err, fs.ErrNotExist))
	}
	switch {
	case err != nil:
		a.show(buildErrText(err), ui.Error, 8*time.Second)
	case b != nil:
		a.show(i18n.Tf("build loaded: %s", b.Summary()), ui.Info, 6*time.Second)
	}
}
