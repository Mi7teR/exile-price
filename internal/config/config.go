// Package config reads config.toml from the app folder (%APPDATA%\exile-price
// on Windows) and writes the defaults there on first run.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"time"

	"github.com/BurntSushi/toml"
)

type Config struct {
	League    string        `toml:"league"`     // empty = current temporary softcore league
	POESESSID string        `toml:"poesessid"`  // optional pathofexile.com session; never logged
	HideAfter time.Duration `toml:"hide_after"` // the answer line hides after this
	RollSlack float64       `toml:"roll_slack"` // rare filters: min = roll × (1 − roll_slack)
	Fixers    Fixers        `toml:"fixers"`

	BuildSource string `toml:"build_source"` // "pob", "clipboard" or empty = no build
	PoBBuild    string `toml:"pob_build"`    // the PoB build file used when build_source = "pob"
	PoBDir      string `toml:"pob_dir"`      // the PoB builds folder the file picker opens in
}

// Values of Config.BuildSource.
const (
	SourcePoB       = "pob"
	SourceClipboard = "clipboard"
)

type Fixers struct {
	CheapRatio        float64       `toml:"cheap_ratio"`
	StaleAge          time.Duration `toml:"stale_age"`
	MinReferenceChaos float64       `toml:"min_reference_chaos"`
}

func Default() Config {
	return Config{
		HideAfter: 5 * time.Second,
		RollSlack: 0.15,
		Fixers:    Fixers{CheapRatio: 0.5, StaleAge: 24 * time.Hour, MinReferenceChaos: 10},
	}
}

const header = `# exile-price settings. Delete a line to get its default back.
# league = ""  → current temporary softcore league; set e.g. "Hardcore Covens" to override.
# poesessid    → optional, the POESESSID cookie of pathofexile.com; keep this file private.
# build_source → where the build for item comparison comes from: "pob" (a Path of Building save file) or "clipboard" (a PoB code, kept in build.txt); empty = none.
# pob_build    → the Path of Building build file (.xml) read when build_source = "pob".
# pob_dir      → the Path of Building builds folder the file picker starts in.
`

// Dir is the app folder; it is created if missing.
func Dir() (string, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return appDir(base)
}

// appDir returns base/exile-price. The app was called poe-pricecheck before;
// on the first run after the rename its folder (settings, log, cache) is moved
// over, unless the new folder already exists. If the move fails (the old
// exe still runs and holds its log open), the old folder is used this time
// and the move is tried again on the next run.
func appDir(base string) (string, error) {
	dir := filepath.Join(base, "exile-price")
	if _, err := os.Stat(dir); errors.Is(err, fs.ErrNotExist) {
		legacy := filepath.Join(base, "poe-pricecheck")
		if fi, err := os.Stat(legacy); err == nil && fi.IsDir() {
			if os.Rename(legacy, dir) != nil {
				return legacy, nil
			}
		}
	}
	return dir, os.MkdirAll(dir, 0o755)
}

// Load reads dir/config.toml over the defaults, writing the file when absent.
func Load(dir string) (Config, error) {
	cfg := Default()
	path := filepath.Join(dir, "config.toml")
	b, err := os.ReadFile(path)
	if errors.Is(err, fs.ErrNotExist) {
		return cfg, Save(dir, cfg)
	}
	if err != nil {
		return cfg, err
	}
	if _, err := toml.Decode(string(b), &cfg); err != nil {
		return Default(), fmt.Errorf("%s: %w", path, err)
	}
	return cfg, nil
}

// Save writes cfg to dir/config.toml, replacing it atomically. The file may
// hold the POESESSID: mode 0600 guards it on Unix; on Windows it is guarded
// only by living in the user's own %APPDATA%.
func Save(dir string, cfg Config) error {
	var buf bytes.Buffer
	buf.WriteString(header)
	if err := toml.NewEncoder(&buf).Encode(cfg); err != nil {
		return err
	}
	path := filepath.Join(dir, "config.toml")
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
