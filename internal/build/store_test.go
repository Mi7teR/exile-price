package build

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writeAt(t *testing.T, path string, data []byte, mod time.Time) {
	t.Helper()
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mod, mod); err != nil {
		t.Fatal(err)
	}
}

func pobFile(t *testing.T) (*Store, string, time.Time) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "a.xml")
	mod := time.Now().Add(-time.Hour)
	writeAt(t, path, read(t, "builds", "assassin.xml"), mod)
	s := NewStore(t.TempDir(), calc)
	if _, err := s.Use("pob", path); err != nil {
		t.Fatal(err)
	}
	return s, path, mod
}

func TestStoreClipboardRoundTrip(t *testing.T) {
	dir := t.TempDir()
	code := read(t, "builds", "assassin.code")
	b, err := NewStore(dir, calc).SaveClipboard(string(code))
	if err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "build.txt"))
	if err != nil || string(got) != string(code) {
		t.Fatalf("build.txt: %v, same=%v", err, string(got) == string(code))
	}
	s2 := NewStore(dir, calc)
	b2, err := s2.Use("clipboard", "")
	if err != nil || b2 == nil || b2.Class != b.Class {
		t.Fatalf("Use = %v, %v", b2, err)
	}
	if cur, err := s2.Current(); err != nil || cur != b2 {
		t.Fatalf("Current = %v, %v", cur, err)
	}
}

func TestStoreClipboardRejectsGarbage(t *testing.T) {
	dir := t.TempDir()
	s := NewStore(dir, calc)
	if _, err := s.SaveClipboard(string(read(t, "builds", "assassin.code"))); err != nil {
		t.Fatal(err)
	}
	before, _ := s.Current()
	if _, err := s.SaveClipboard("hello"); err == nil {
		t.Fatal("want error")
	}
	if cur, _ := s.Current(); cur != before {
		t.Fatal("current build changed")
	}
	got, _ := os.ReadFile(filepath.Join(dir, "build.txt"))
	if string(got) != string(read(t, "builds", "assassin.code")) {
		t.Fatal("build.txt overwritten")
	}

	s2 := NewStore(t.TempDir(), calc)
	if _, err := s2.SaveClipboard("hello"); err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(filepath.Join(s2.appDir, "build.txt")); err == nil {
		t.Fatal("build.txt created")
	}
}

func TestStorePoBReloadsOnChange(t *testing.T) {
	s, path, mod := pobFile(t)
	if b, err := s.Current(); err != nil || b.Class != "Assassin" {
		t.Fatalf("first = %v, %v", b, err)
	}
	writeAt(t, path, read(t, "builds", "inquisitor_swap.code"), mod.Add(time.Minute))
	b, err := s.Current()
	if err != nil || b == nil || b.Class != "Inquisitor" {
		t.Fatalf("after change = %v, %v", b, err)
	}
}

func TestStorePoBKeepsOldOnBrokenWrite(t *testing.T) {
	s, path, mod := pobFile(t)
	old, _ := s.Current()
	xml := read(t, "builds", "assassin.xml")
	writeAt(t, path, xml[:len(xml)/2], mod.Add(time.Minute))
	b, err := s.Current()
	if err == nil || b != old {
		t.Fatalf("broken = %v, %v", b, err)
	}
	// the same broken stamp: same answer, no new load
	if b, err := s.Current(); err == nil || b != old {
		t.Fatalf("repeat = %v, %v", b, err)
	}
	writeAt(t, path, read(t, "builds", "inquisitor_swap.code"), mod.Add(2*time.Minute))
	b, err = s.Current()
	if err != nil || b.Class != "Inquisitor" {
		t.Fatalf("fixed = %v, %v", b, err)
	}
}

func TestStorePoBFileGone(t *testing.T) {
	s, path, _ := pobFile(t)
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if b, err := s.Current(); b != nil || err == nil {
		t.Fatalf("gone = %v, %v", b, err)
	}
}

func TestStoreNone(t *testing.T) {
	s := NewStore(t.TempDir(), calc)
	if b, err := s.Use("", ""); b != nil || err != nil {
		t.Fatalf("Use = %v, %v", b, err)
	}
	if b, err := s.Current(); b != nil || err != nil {
		t.Fatalf("Current = %v, %v", b, err)
	}
}

func TestStoreClipboardRenameFailRemovesTmp(t *testing.T) {
	dir := t.TempDir()
	// build.txt is a folder with a file in it: the rename onto it fails.
	if err := os.MkdirAll(filepath.Join(dir, "build.txt", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := NewStore(dir, calc).SaveClipboard(string(read(t, "builds", "assassin.code"))); err == nil {
		t.Fatal("want error")
	}
	if _, err := os.Stat(filepath.Join(dir, "build.txt.tmp")); !os.IsNotExist(err) {
		t.Fatalf("tmp left behind: %v", err)
	}
}
