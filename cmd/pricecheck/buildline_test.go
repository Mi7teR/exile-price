package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"testing"

	"github.com/Mi7teR/exile-price/internal/build"
	"github.com/Mi7teR/exile-price/internal/config"
	"github.com/Mi7teR/exile-price/internal/ui"
)

func TestSubSpans(t *testing.T) {
	got := subSpans([]build.Segment{
		{Text: "instead of Ring 2: "},
		{Text: "Life +68", Tone: build.Better},
		{Text: " · Fire -12%", Tone: build.Worse},
	})
	want := []ui.Span{
		{Text: "instead of Ring 2: ", Kind: ui.Muted},
		{Text: "Life +68", Kind: ui.Better},
		{Text: " · Fire -12%", Kind: ui.Worse},
	}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestWithBuild(t *testing.T) {
	old := config.Config{League: "L", BuildSource: config.SourcePoB, PoBBuild: `C:\b\old.xml`, PoBDir: `C:\custom`}
	cases := []struct {
		name string
		c    ui.BuildChoice
		want [3]string // source, pob_build, pob_dir
	}{
		{"pob file", ui.BuildChoice{Dir: `C:\b`, Source: "pob", Path: `C:\b\new.xml`}, [3]string{"pob", `C:\b\new.xml`, ""}},
		{"custom dir kept", ui.BuildChoice{Dir: `D:\pob`, Source: "pob", Path: `D:\pob\x.xml`}, [3]string{"pob", `D:\pob\x.xml`, `D:\pob`}},
		{"clipboard keeps the file", ui.BuildChoice{Dir: `C:\b`, Source: "clipboard"}, [3]string{"clipboard", `C:\b\old.xml`, ""}},
		{"none", ui.BuildChoice{Dir: `C:\b`}, [3]string{"", `C:\b\old.xml`, ""}},
	}
	for _, tc := range cases {
		got := withBuild(old, tc.c, `C:\b`)
		if g := [3]string{got.BuildSource, got.PoBBuild, got.PoBDir}; g != tc.want || got.League != "L" {
			t.Errorf("%s: got %q league %q, want %q", tc.name, g, got.League, tc.want)
		}
	}
}

func TestBuildErrText(t *testing.T) {
	if got := buildErrText(fmt.Errorf("open: %w", fs.ErrNotExist)); got != "build file not found" {
		t.Errorf("missing file: %q", got)
	}
	if got := buildErrText(errors.New("bad xml")); got != "build does not compute: bad xml" {
		t.Errorf("other: %q", got)
	}
}

func TestClipboardErrText(t *testing.T) {
	err := errors.New("x")
	if got := clipboardErrText("hello there", err); got != "no PoB code on the clipboard" {
		t.Errorf("plain text: %q", got)
	}
	code, rerr := os.ReadFile("../../testdata/builds/assassin.code")
	if rerr != nil {
		t.Fatal(rerr)
	}
	if got := clipboardErrText(string(code), err); got != "build does not compute: x" {
		t.Errorf("valid code: %q", got)
	}
	if got := clipboardErrText(" <PathOfBuilding/>", err); got != "build does not compute: x" {
		t.Errorf("xml: %q", got)
	}
}
