package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadWritesDefaultsThenReadsEdits(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg != Default() {
		t.Fatalf("first load = %+v, want defaults", cfg)
	}
	b, err := os.ReadFile(filepath.Join(dir, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("written:\n%s", b)

	edited := "league = \"Hardcore Covens\"\nhide_after = \"8s\"\n[fixers]\nmin_reference_chaos = 25\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.League != "Hardcore Covens" || cfg.HideAfter != 8*time.Second || cfg.Fixers.MinReferenceChaos != 25 {
		t.Errorf("edited load = %+v", cfg)
	}
	if cfg.Fixers.CheapRatio != 0.5 || cfg.RollSlack != 0.15 {
		t.Errorf("missing keys lost their defaults: %+v", cfg)
	}
}

func TestSaveRoundTrip(t *testing.T) {
	dir := t.TempDir()
	cfg := Default()
	cfg.League = "Hardcore Covens"
	cfg.POESESSID = "0123456789abcdef0123456789abcdef"
	if err := Save(dir, cfg); err != nil {
		t.Fatal(err)
	}
	got, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if got != cfg {
		t.Errorf("round trip: got %+v, want %+v", got, cfg)
	}
}

func TestAppDirMovesLegacyFolder(t *testing.T) {
	base := t.TempDir()
	old := filepath.Join(base, "poe-pricecheck")
	if err := os.MkdirAll(old, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(old, "config.toml"), []byte(`league = "Hardcore"`), 0o600); err != nil {
		t.Fatal(err)
	}
	dir, err := appDir(base)
	if err != nil {
		t.Fatal(err)
	}
	if dir != filepath.Join(base, "exile-price") {
		t.Fatalf("dir = %s", dir)
	}
	cfg, err := Load(dir)
	if err != nil || cfg.League != "Hardcore" {
		t.Fatalf("league = %q, err = %v; want the moved config", cfg.League, err)
	}
	if _, err := os.Stat(old); !os.IsNotExist(err) {
		t.Fatalf("old folder still there: %v", err)
	}
}

func TestAppDirKeepsExistingFolder(t *testing.T) {
	base := t.TempDir()
	for _, name := range []string{"poe-pricecheck", "exile-price"} {
		if err := os.MkdirAll(filepath.Join(base, name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := appDir(base); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(base, "poe-pricecheck")); err != nil {
		t.Fatalf("old folder must stay when the new one exists: %v", err)
	}
}

func TestLoadReadsBuildKeys(t *testing.T) {
	dir := t.TempDir()
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BuildSource != "" || cfg.PoBBuild != "" || cfg.PoBDir != "" {
		t.Fatalf("defaults = %+v", cfg)
	}
	edited := "build_source = \"pob\"\npob_build = 'C:\\x\\a.xml'\npob_dir = 'C:\\x'\n"
	if err := os.WriteFile(filepath.Join(dir, "config.toml"), []byte(edited), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.BuildSource != SourcePoB || cfg.PoBBuild != `C:\x\a.xml` || cfg.PoBDir != `C:\x` {
		t.Errorf("edited load = %+v", cfg)
	}
}

func TestLoadRaisesOldDefaultHideAfter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")
	old := Default()
	old.HideAfter = 5 * time.Second // the default before 30 s, saved by an older version
	if err := Save(dir, old); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.HideAfter != 30*time.Second {
		t.Errorf("hide_after = %v, want 30s", cfg.HideAfter)
	}
	again, err := Load(dir) // written back: a 5 s set later by hand stays
	if err != nil || again.HideAfter != 30*time.Second {
		t.Errorf("not saved: %v, %v", again.HideAfter, err)
	}
	if err := os.WriteFile(path, []byte("hide_after = \"8s\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if cfg, _ := Load(dir); cfg.HideAfter != 8*time.Second {
		t.Errorf("own value changed: %v", cfg.HideAfter)
	}
}
