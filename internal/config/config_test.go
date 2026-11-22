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
