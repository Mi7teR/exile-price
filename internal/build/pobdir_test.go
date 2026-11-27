package build

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildsDirDefault(t *testing.T) {
	u := t.TempDir()
	if got, want := buildsDir(u), filepath.Join(u, "Builds"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildsDirFromSettings(t *testing.T) {
	u := t.TempDir()
	xml := `<PathOfBuilding><Mode mode="LIST"/><Misc buildSortMode="NAME" buildPath="D:/PoB/MyBuilds/"/></PathOfBuilding>`
	if err := os.WriteFile(filepath.Join(u, "Settings.xml"), []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := buildsDir(u), "D:/PoB/MyBuilds/"; got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestBuildsDirBrokenSettings(t *testing.T) {
	u := t.TempDir()
	if err := os.WriteFile(filepath.Join(u, "Settings.xml"), []byte("<<garbage"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, want := buildsDir(u), filepath.Join(u, "Builds"); got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestListBuilds(t *testing.T) {
	d := t.TempDir()
	if err := os.MkdirAll(filepath.Join(d, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	files := map[string]time.Time{
		"a.xml":     time.Now().Add(-2 * time.Hour),
		"sub/b.xml": time.Now().Add(-time.Hour),
		"notes.txt": time.Now(),
	}
	for name, mt := range files {
		p := filepath.Join(d, filepath.FromSlash(name))
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	got := ListBuilds(d)
	if len(got) != 2 || got[0].Name != "sub/b" || got[1].Name != "a" {
		t.Fatalf("got %+v", got)
	}
	if got[0].Path != filepath.Join(d, "sub", "b.xml") {
		t.Errorf("path %q", got[0].Path)
	}
}

func TestListBuildsMissingDir(t *testing.T) {
	if got := ListBuilds(filepath.Join(t.TempDir(), "nope")); len(got) != 0 {
		t.Errorf("got %+v", got)
	}
}

func TestDefaultDirFrom(t *testing.T) {
	if got := defaultDirFrom(""); got != "" {
		t.Errorf("empty userPath: got %q", got)
	}
	u := t.TempDir()
	if got := defaultDirFrom(u); got != "" {
		t.Errorf("no Builds: got %q", got)
	}
	builds := filepath.Join(u, "Builds")
	if err := os.Mkdir(builds, 0o755); err != nil {
		t.Fatal(err)
	}
	missing := filepath.Join(u, "gone")
	xml := `<PathOfBuilding><Misc buildPath="` + missing + `"/></PathOfBuilding>`
	if err := os.WriteFile(filepath.Join(u, "Settings.xml"), []byte(xml), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := defaultDirFrom(u); got != builds {
		t.Errorf("fallback: got %q, want %q", got, builds)
	}
}
