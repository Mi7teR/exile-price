// Package build loads a Path of Building build and compares an item against it.
package build

import (
	"bytes"
	"errors"
	"fmt"
	"strings"
	"sync"

	"github.com/Mi7teR/exile-calc/engine"
	"github.com/Mi7teR/exile-calc/pobcode"
)

// ErrNoSlot means the item fits no slot of the build.
var ErrNoSlot = errors.New("build: the item fits no slot of the build")

// Build is a loaded PoB build ready for item comparisons.
type Build struct {
	Class string
	Level int

	cmp  *engine.Comparer
	swap bool

	// Summary's stats, computed on first use: a reload after a PoB save
	// needs only the Comparer.
	calc *engine.Calculator
	pb   *pobcode.Build
	once sync.Once
	base map[string]float64
}

// Delta is the change a build gets from equipping an item.
type Delta struct {
	Slots          []string // the slot the item goes to, then other slots it empties
	Empty          bool     // the slot it goes to was empty
	Before, Change map[string]float64
	Partial        bool     // Unparsed or Incomplete are not empty
	Missing        []string // the Unparsed and Incomplete lines, for the log
}

// Load reads a PoB code or build XML (first non-space byte '<').
func Load(calc *engine.Calculator, data []byte) (*Build, error) {
	data = bytes.TrimSpace(data)
	var pb *pobcode.Build
	var err error
	if len(data) > 0 && data[0] == '<' {
		pb, err = pobcode.Parse(data)
	} else {
		pb, err = pobcode.FromCode(string(data))
	}
	if err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	cmp, err := calc.NewComparer(pb)
	if err != nil {
		return nil, fmt.Errorf("build: %w", err)
	}
	b := &Build{Class: pb.AscendClassName, Level: pb.Level, cmp: cmp, calc: calc, pb: pb}
	if b.Class == "" {
		b.Class = pb.ClassName
	}
	if set := pb.ActiveItems(); set != nil {
		b.swap = set.UseSecondWeaponSet
	}
	return b, nil
}

// dpsKey is the DPS stat to compare by: FullDPS when the build uses it.
func dpsKey(before map[string]float64) string {
	if before["FullDPS"] > 0 {
		return "FullDPS"
	}
	return "CombinedDPS"
}

// Summary is a one-line description such as "Assassin 98 · DPS 3.16M · Life 4658".
func (b *Build) Summary() string {
	b.once.Do(func() {
		if res, err := b.calc.Calc(b.pb); err == nil {
			b.base = res.Stats
		}
	})
	parts := []string{fmt.Sprintf("%s %d", b.Class, b.Level)}
	if dps := b.base[dpsKey(b.base)]; dps > 0 {
		parts = append(parts, "DPS "+shortNum(dps))
	}
	if life := b.base["Life"]; life > 0 {
		parts = append(parts, fmt.Sprintf("Life %.0f", life))
	}
	return strings.Join(parts, " · ")
}

func shortNum(v float64) string {
	switch {
	case v >= 1e6:
		return fmt.Sprintf("%.2fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.1fK", v/1e3)
	}
	return fmt.Sprintf("%.0f", v)
}

// Compare equips the item into each candidate slot and returns the better outcome.
func (b *Build) Compare(itemText string) (*Delta, error) {
	var best *engine.ItemComparison
	var bestSlot string
	for _, slot := range slotsFor(itemClass(itemText), b.swap) {
		c, err := b.cmp.Item(slot, itemText)
		if errors.Is(err, engine.ErrItemNotValidForSlot) || errors.Is(err, engine.ErrSlotNotAvailable) {
			continue
		}
		if err != nil {
			return nil, fmt.Errorf("build: slot %s: %w", slot, err)
		}
		if best == nil || better(c, best) {
			best, bestSlot = c, slot
		}
	}
	if best == nil {
		return nil, ErrNoSlot
	}
	d := &Delta{
		Before:  best.Before,
		Change:  best.Delta,
		Partial: len(best.Unparsed)+len(best.Incomplete) > 0,
	}
	d.Slots, d.Empty = slotsLabel(bestSlot, best.Replaced)
	d.Missing = append(append(d.Missing, best.Unparsed...), best.Incomplete...)
	return d, nil
}

// slotsLabel puts the slot the item goes to first, then the other slots it
// empties (a two-hander empties "Weapon 2"); empty says the slot itself held
// nothing. Replaced names only slots that held an item.
func slotsLabel(slot string, replaced []string) (slots []string, empty bool) {
	slots, empty = []string{slot}, true
	for _, r := range replaced {
		if r == slot {
			empty = false
		} else {
			slots = append(slots, r)
		}
	}
	return slots, empty
}

// Equippable reports whether the item can go to some gear slot: rings,
// amulets, belts, armour, weapons, shields and quivers.
func Equippable(itemText string) bool {
	return slotsFor(itemClass(itemText), false) != nil
}

// better reports whether a beats b: larger relative DPS change, then larger EHP change.
func better(a, b *engine.ItemComparison) bool {
	da, db := dpsScore(a), dpsScore(b)
	if da != db {
		return da > db
	}
	return a.Delta["TotalEHP"] > b.Delta["TotalEHP"]
}

func dpsScore(c *engine.ItemComparison) float64 {
	k := dpsKey(c.Before)
	if base := c.Before[k]; base > 0 {
		return c.Delta[k] / base
	}
	return c.Delta[k]
}

func itemClass(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), "Item Class:"); ok {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// slotsFor lists the PoB slots an item of the given class may go to.
func slotsFor(class string, swap bool) []string {
	var slots []string
	switch class {
	case "Rings":
		return []string{"Ring 1", "Ring 2", "Ring 3"}
	case "Amulets":
		return []string{"Amulet"}
	case "Belts":
		return []string{"Belt"}
	case "Helmets":
		return []string{"Helmet"}
	case "Body Armours":
		return []string{"Body Armour"}
	case "Gloves":
		return []string{"Gloves"}
	case "Boots":
		return []string{"Boots"}
	case "Claws", "Daggers", "Rune Daggers", "Wands", "One Hand Swords", "One Hand Axes",
		"One Hand Maces", "Thrusting One Hand Swords", "Sceptres":
		slots = []string{"Weapon 1", "Weapon 2"}
	case "Bows", "Staves", "Warstaves", "Two Hand Swords", "Two Hand Axes", "Two Hand Maces", "Fishing Rods":
		slots = []string{"Weapon 1"}
	case "Shields", "Quivers":
		slots = []string{"Weapon 2"}
	default:
		return nil
	}
	if swap {
		for i, s := range slots {
			slots[i] = s + " Swap"
		}
	}
	return slots
}
