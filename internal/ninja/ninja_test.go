package ninja

import (
	"os"
	"testing"
)

func load(t *testing.T) *Index {
	t.Helper()
	f, err := os.Open("../../testdata/recorded/ninja-overviews.json")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	idx, err := Parse(f)
	if err != nil {
		t.Fatal(err)
	}
	return idx
}

func TestLookups(t *testing.T) {
	idx := load(t)
	cases := []struct {
		name string
		got  func() (Line, bool)
		want string // variant
	}{
		{"gem exact", func() (Line, bool) { return idx.Gem("Arc", 20, 20, false) }, "20/20"},
		{"gem corrupted", func() (Line, bool) { return idx.Gem("Arc", 20, 20, true) }, "20/20c"},
		{"gem no quality", func() (Line, bool) { return idx.Gem("Arc", 20, 0, false) }, "20"},
		{"gem fallback quality", func() (Line, bool) { return idx.Gem("Arc", 21, 7, true) }, "21c"},
		{"unique 6L", func() (Line, bool) { return idx.Unique("Shavronne's Wrappings", "Occultist's Vestment", 6) }, "Occultist's Vestment, 6L"},
		{"unique unlinked", func() (Line, bool) { return idx.Unique("Shavronne's Wrappings", "Occultist's Vestment", 2) }, "Occultist's Vestment"},
		{"currency", func() (Line, bool) { return idx.Best("Divine Orb") }, ""},
	}
	for _, c := range cases {
		l, ok := c.got()
		if !ok || l.Variant != c.want {
			t.Errorf("%s: got %+v ok=%v, want variant %q", c.name, l, ok, c.want)
		}
	}
	if c, ok := idx.Chaos("Divine Orb"); !ok || c < 50 {
		t.Errorf("divine = %v %v", c, ok)
	}
}
