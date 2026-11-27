package main

import (
	"errors"
	"fmt"
	"io/fs"
	"testing"
)

func TestBuildIssueMissingFileShownOnce(t *testing.T) {
	var b buildIssue
	gone := fmt.Errorf("stat x.xml: %w", fs.ErrNotExist)
	if !b.failed(gone) {
		t.Error("first error not reported for the log")
	}
	text, isGone := b.take()
	if text != "build file not found" || !isGone {
		t.Fatalf("take = %q, %v", text, isGone)
	}
	b.shown(isGone)
	if b.failed(gone) {
		t.Error("same error logged twice")
	}
	if text, _ := b.take(); text != "" {
		t.Errorf("missing file shown again: %q", text)
	}
	b.ok() // the file is back
	b.failed(gone)
	if text, _ := b.take(); text != "build file not found" {
		t.Errorf("missing again after it came back: %q", text)
	}
}

func TestBuildIssuePutBack(t *testing.T) {
	var b buildIssue
	b.set("engine failed", false)
	text, gone := b.take()
	b.putBack(text, gone) // the line was gone: try under the next one
	if got, _ := b.take(); got != "engine failed" {
		t.Fatalf("note lost: %q", got)
	}
	b.set("newer", false)
	b.putBack("older", false)
	if got, _ := b.take(); got != "newer" {
		t.Errorf("older note replaced a newer one: %q", got)
	}
}

func TestBuildIssueBrokenFileNoNote(t *testing.T) {
	var b buildIssue
	if !b.failed(errors.New("xml: bad")) {
		t.Error("not logged")
	}
	if text, _ := b.take(); text != "" {
		t.Errorf("a half-written file must keep the old build silently, got note %q", text)
	}
}
