// Command pricecheck-cli prices one item text from stdin or a file and prints
// the same line the overlay shows.
//
//	pricecheck-cli < testdata/items/boots_rare.txt
//	pricecheck-cli -v testdata/items/unique_goldrim.txt
package main

import (
	"cmp"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Mi7teR/exile-calc/engine"
	"github.com/Mi7teR/exile-price/internal/build"
	"github.com/Mi7teR/exile-price/internal/config"
	"github.com/Mi7teR/exile-price/internal/pricer"
)

var version = "0.1.0"

func main() {
	league := flag.String("league", "", "league id; empty = from config, else current temporary softcore league")
	verbose := flag.Bool("v", false, "show lots, dropped lots and timings")
	buildFile := flag.String("build", "", "file with a PoB code or PoB XML; prints how the build changes with the item")
	flag.Parse()
	if err := run(*league, *verbose, *buildFile, flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(league string, verbose bool, buildFile, file string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	text, err := readInput(file)
	if err != nil {
		return err
	}
	// The build change needs no network, so compute it before pricing.
	var delta string
	if buildFile != "" {
		delta, err = buildDelta(buildFile, text, verbose)
		if err != nil {
			return err
		}
	}
	dir, err := config.Dir()
	if err != nil {
		return err
	}
	cfg, err := config.Load(dir)
	if err != nil {
		return err
	}
	if league != "" {
		cfg.League = league
	}
	opts := pricer.Options{
		HTTP:      &http.Client{Timeout: 15 * time.Second},
		UserAgent: "exile-price/" + version + " (github.com/Mi7teR/exile-price)",
		CacheDir:  filepath.Join(dir, "cache"),
		POESESSID: cmp.Or(os.Getenv("POESESSID"), cfg.POESESSID),
		Config:    cfg,
	}
	t0 := time.Now()
	p, err := pricer.Load(ctx, opts)
	if err != nil {
		printDelta(delta)
		return err
	}
	startup := time.Since(t0)

	t1 := time.Now()
	r, err := p.Price(ctx, text)
	if err != nil {
		printDelta(delta)
		return err
	}
	took := time.Since(t1)

	divine := p.Divine()
	fmt.Println(r.Line(divine))
	printDelta(delta)
	if verbose {
		fmt.Printf("\n%s · лига %s · всего на сайте %d\n", r.Item, p.League, r.Total)
		if r.Note != "" {
			fmt.Println(r.Note)
		}
		for _, l := range r.Kept {
			fmt.Printf("  %-10s %-28s %s назад\n", pricer.Money(l.Chaos, divine), l.Account, ago(l.Indexed))
		}
		for _, d := range r.Dropped {
			fmt.Printf("  ✕ %-8s %-28s %s назад — %s\n", pricer.Money(d.Lot.Chaos, divine), d.Lot.Account, ago(d.Lot.Indexed), d.Reason)
		}
		if r.Link != "" {
			fmt.Println("поиск:", r.Link)
		}
		fmt.Printf("время: старт %s, прайсчек %s", startup.Round(time.Millisecond), took.Round(time.Millisecond))
		for _, t := range r.Timings {
			fmt.Printf(" · %s %s", t.Step, t.Took.Round(time.Millisecond))
		}
		fmt.Println()
	}
	return nil
}

func printDelta(delta string) {
	if delta != "" {
		fmt.Println(delta)
	}
}

// buildDelta returns the build change line for the item, "" when the item fits
// no slot. A build that cannot be compared is reported to stderr, not fatal.
func buildDelta(buildFile, itemText string, verbose bool) (string, error) {
	data, err := os.ReadFile(buildFile)
	if err != nil {
		return "", err
	}
	calc, err := engine.New()
	if err != nil {
		return "", err
	}
	b, err := build.Load(calc, data)
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		return "", nil
	}
	d, err := b.Compare(itemText)
	if errors.Is(err, build.ErrNoSlot) {
		return "", nil
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "build:", err)
		return "", nil
	}
	if verbose {
		for _, m := range d.Missing {
			fmt.Fprintln(os.Stderr, "  missing:", m)
		}
	}
	return d.String(), nil
}

func ago(t time.Time) string {
	d := time.Since(t)
	if d >= 24*time.Hour {
		return fmt.Sprintf("%dд", int(d.Hours()/24))
	}
	return fmt.Sprintf("%dч", int(d.Hours()))
}

func readInput(file string) (string, error) {
	var b []byte
	var err error
	if file == "" || file == "-" {
		b, err = io.ReadAll(os.Stdin)
	} else {
		b, err = os.ReadFile(file)
	}
	return string(b), err
}
