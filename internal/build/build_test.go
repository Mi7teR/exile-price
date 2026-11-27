package build

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/Mi7teR/exile-calc/engine"
	"github.com/Mi7teR/exile-calc/pobcode"

	"github.com/Mi7teR/exile-price/internal/i18n"
)

var calc *engine.Calculator

func TestMain(m *testing.M) {
	var err error
	calc, err = engine.New()
	if err != nil {
		panic(err)
	}
	os.Exit(m.Run())
}

func read(t *testing.T, parts ...string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(append([]string{"..", "..", "testdata"}, parts...)...))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func load(t *testing.T, name string) *Build {
	t.Helper()
	b, err := Load(calc, read(t, "builds", name))
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func TestLoadCodeAndXMLAgree(t *testing.T) {
	c, x := load(t, "assassin.code"), load(t, "assassin.xml")
	if c.Class != "Assassin" || c.Level != 98 {
		t.Fatalf("code: %q %d", c.Class, c.Level)
	}
	if x.Class != c.Class || x.Level != c.Level {
		t.Fatalf("xml: %q %d", x.Class, x.Level)
	}
	if c.Summary() != x.Summary() {
		t.Fatalf("summary differs: %q vs %q", c.Summary(), x.Summary())
	}
	t.Log(c.Summary())
}

func TestLoadRejectsGarbage(t *testing.T) {
	if _, err := Load(calc, []byte("hello")); err == nil {
		t.Fatal("want error")
	}
}

func TestCompareRingPicksBetterSlot(t *testing.T) {
	b := load(t, "assassin.code")
	d, err := b.Compare(string(read(t, "items", "real_ring_rare_crafted.txt")))
	if err != nil {
		t.Fatal(err)
	}
	if d.Empty {
		t.Error("Ring 2 holds an item, Empty set")
	}
	if !reflect.DeepEqual(d.Slots, []string{"Ring 2"}) {
		t.Fatalf("slots %v", d.Slots)
	}
	for k, want := range map[string]float64{"Life": -49, "EnergyShield": 37, "FireResist": 4} {
		if d.Change[k] != want {
			t.Errorf("%s = %v, want %v", k, d.Change[k], want)
		}
	}
	if d.Before["Life"] == 0 {
		t.Error("Before empty")
	}
}

func TestCompareNoSlot(t *testing.T) {
	b := load(t, "assassin.code")
	for _, f := range []string{"gem_arc.txt", "real_abyss_jewel.txt"} {
		_, err := b.Compare(string(read(t, "items", f)))
		if !errors.Is(err, ErrNoSlot) {
			t.Errorf("%s: %v", f, err)
		}
	}
}

func TestSlotsFor(t *testing.T) {
	tests := []struct {
		class string
		swap  bool
		want  []string
	}{
		{"Rings", false, []string{"Ring 1", "Ring 2", "Ring 3"}},
		{"Bows", false, []string{"Weapon 1"}},
		{"Claws", false, []string{"Weapon 1", "Weapon 2"}},
		{"Shields", false, []string{"Weapon 2"}},
		{"Claws", true, []string{"Weapon 1 Swap", "Weapon 2 Swap"}},
		{"Jewels", false, nil},
		{"Life Flasks", false, nil},
	}
	for _, tt := range tests {
		got := slotsFor(tt.class, tt.swap)
		if len(got) != len(tt.want) || (len(got) > 0 && !reflect.DeepEqual(got, tt.want)) {
			t.Errorf("slotsFor(%q,%v) = %v, want %v", tt.class, tt.swap, got, tt.want)
		}
	}
}

func TestCompareSwapBuildUsesSwapSlots(t *testing.T) {
	b := load(t, "inquisitor_swap.code")
	d, err := b.Compare(string(read(t, "items", "real_bow_rare_advanced.txt")))
	if err != nil {
		t.Fatal(err)
	}
	if len(d.Slots) == 0 || d.Slots[0] != "Weapon 1 Swap" {
		t.Fatalf("slots %v", d.Slots)
	}
	if len(d.Change) == 0 {
		t.Fatal("empty change")
	}
}

func TestCompareIntoEmptySlot(t *testing.T) {
	pb, err := pobcode.FromCode(string(read(t, "builds", "assassin.code")))
	if err != nil {
		t.Fatal(err)
	}
	set := pb.ActiveItems()
	cleared := false
	for i := range set.Slots {
		if set.Slots[i].Name == "Ring 2" {
			set.Slots[i].ItemID = 0
			cleared = true
		}
	}
	if !cleared {
		t.Fatal("no Ring 2 slot in the fixture")
	}
	code, err := pb.Code()
	if err != nil {
		t.Fatal(err)
	}
	b, err := Load(calc, []byte(code))
	if err != nil {
		t.Fatal(err)
	}
	d, err := b.Compare(string(read(t, "items", "real_ring_rare_crafted.txt")))
	if err != nil {
		t.Fatal(err)
	}
	if !d.Empty || !reflect.DeepEqual(d.Slots, []string{"Ring 2"}) {
		t.Fatalf("empty %v, slots %v", d.Empty, d.Slots)
	}
	setLang(t, i18n.EN)
	if got := d.Segments()[0].Text; got != "into empty Ring 2: " {
		t.Fatalf("prefix %q", got)
	}
}

func TestSlotsLabel(t *testing.T) {
	cases := []struct {
		slot     string
		replaced []string
		slots    []string
		empty    bool
	}{
		{"Ring 2", []string{"Ring 2"}, []string{"Ring 2"}, false},
		{"Ring 3", nil, []string{"Ring 3"}, true},
		{"Weapon 1", []string{"Weapon 1", "Weapon 2"}, []string{"Weapon 1", "Weapon 2"}, false},
		// a bow into an empty main hand beside a shield: the label names the main hand first
		{"Weapon 1", []string{"Weapon 2"}, []string{"Weapon 1", "Weapon 2"}, true},
	}
	for _, c := range cases {
		slots, empty := slotsLabel(c.slot, c.replaced)
		if !reflect.DeepEqual(slots, c.slots) || empty != c.empty {
			t.Errorf("slotsLabel(%q, %v) = %v, %v; want %v, %v", c.slot, c.replaced, slots, empty, c.slots, c.empty)
		}
	}
}

func TestEquippable(t *testing.T) {
	for file, want := range map[string]bool{
		"real_ring_rare_crafted.txt": true,
		"boots_rare.txt":             true,
		"currency_divine.txt":        false,
		"gem_arc.txt":                false,
		"real_abyss_jewel.txt":       false,
	} {
		text, err := os.ReadFile(filepath.Join("..", "..", "testdata", "items", file))
		if err != nil {
			t.Fatal(err)
		}
		if got := Equippable(string(text)); got != want {
			t.Errorf("%s: Equippable = %v, want %v", file, got, want)
		}
	}
}
