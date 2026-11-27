package main

import (
	"errors"
	"io/fs"
	"strings"

	"github.com/Mi7teR/exile-calc/pobcode"

	"github.com/Mi7teR/exile-price/internal/build"
	"github.com/Mi7teR/exile-price/internal/config"
	"github.com/Mi7teR/exile-price/internal/i18n"
	"github.com/Mi7teR/exile-price/internal/ui"
)

// subSpans colors the build change line: the prefix is muted, each stat
// green or red.
func subSpans(segs []build.Segment) []ui.Span {
	out := make([]ui.Span, len(segs))
	for i, s := range segs {
		k := ui.Muted
		switch s.Tone {
		case build.Better:
			k = ui.Better
		case build.Worse:
			k = ui.Worse
		}
		out[i] = ui.Span{Text: s.Text, Kind: k}
	}
	return out
}

// withBuild returns cfg with the build choice from the settings form. The
// folder is stored empty when it is the one found automatically.
func withBuild(cfg config.Config, c ui.BuildChoice, defaultDir string) config.Config {
	cfg.BuildSource = c.Source
	if c.Source == config.SourcePoB {
		cfg.PoBBuild = c.Path
	}
	cfg.PoBDir = c.Dir
	if c.Dir == defaultDir {
		cfg.PoBDir = ""
	}
	return cfg
}

// buildEntries lists the PoB builds in dir for the settings form.
func buildEntries(dir string) []ui.BuildEntry {
	var out []ui.BuildEntry
	for _, e := range build.ListBuilds(dir) {
		out = append(out, ui.BuildEntry{Name: e.Name, Path: e.Path})
	}
	return out
}

// buildErrText is the overlay line for a build that failed to load.
func buildErrText(err error) string {
	if errors.Is(err, fs.ErrNotExist) {
		return i18n.T("build file not found")
	}
	return i18n.Tf("build does not compute: %s", err)
}

// engineErrText is the overlay line when the build engine failed to start.
func engineErrText(err error) string {
	return i18n.Tf("build comparison is unavailable: %s", err)
}

// clipboardErrText is the overlay line for a clipboard text that did not
// load as a build: not a PoB code at all, or a code the engine rejects.
func clipboardErrText(text string, err error) string {
	text = strings.TrimSpace(text)
	if !strings.HasPrefix(text, "<") {
		if _, derr := pobcode.Decode(text); derr != nil {
			return i18n.T("no PoB code on the clipboard")
		}
	}
	return i18n.Tf("build does not compute: %s", err)
}
