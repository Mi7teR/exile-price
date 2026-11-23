// Command pricecheck-cli prices one item text from stdin or a file and prints
// the same line the overlay shows.
//
//	pricecheck-cli < testdata/items/boots_rare.txt
//	pricecheck-cli -v testdata/items/unique_goldrim.txt
package main

import (
	"cmp"
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"github.com/Mi7teR/poe-pricecheck/internal/config"
	"github.com/Mi7teR/poe-pricecheck/internal/pricer"
)

var version = "0.1.0"

func main() {
	league := flag.String("league", "", "league id; empty = from config, else current temporary softcore league")
	verbose := flag.Bool("v", false, "show lots, dropped lots and timings")
	flag.Parse()
	if err := run(*league, *verbose, flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(league string, verbose bool, file string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	text, err := readInput(file)
	if err != nil {
		return err
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
		UserAgent: "poe-pricecheck/" + version + " (github.com/Mi7teR/poe-pricecheck)",
		CacheDir:  filepath.Join(dir, "cache"),
		POESESSID: cmp.Or(os.Getenv("POESESSID"), cfg.POESESSID),
		Config:    cfg,
	}
	t0 := time.Now()
	p, err := pricer.Load(ctx, opts)
	if err != nil {
		return err
	}
	startup := time.Since(t0)

	t1 := time.Now()
	r, err := p.Price(ctx, text)
	if err != nil {
		return err
	}
	took := time.Since(t1)

	divine := p.Divine()
	fmt.Println(r.Line(divine))
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
