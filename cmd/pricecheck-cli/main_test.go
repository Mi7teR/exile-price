package main

import (
	"os"
	"strings"
	"testing"
)

func TestBuildDelta(t *testing.T) {
	item, err := os.ReadFile("../../testdata/items/real_ring_rare_crafted.txt")
	if err != nil {
		t.Fatal(err)
	}
	got, err := buildDelta("../../testdata/builds/assassin.code", string(item), false)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "instead of Ring 2:") {
		t.Fatalf("got %q", got)
	}
}
