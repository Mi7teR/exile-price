// Command pricecheck-cli prices one item text from stdin or a file and prints
// the same result the overlay will show.
//
//	pricecheck-cli < testdata/items/boots_rare.txt
//	pricecheck-cli -v testdata/items/unique_goldrim.txt
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mi7teR/poe-lib/gamedata"
	"github.com/Mi7teR/poe-lib/itemtext"
	"github.com/Mi7teR/poe-pricecheck/internal/fixers"
	"github.com/Mi7teR/poe-pricecheck/internal/ninja"
	"github.com/Mi7teR/poe-pricecheck/internal/pricer"
	"github.com/Mi7teR/poe-pricecheck/internal/trade"
)

var version = "0.0.1-spike"

func main() {
	league := flag.String("league", "", "league id; empty = current temporary softcore league")
	verbose := flag.Bool("v", false, "show lots, dropped lots and timings")
	cacheDir := flag.String("cache", defaultCache(), "cache for poe.ninja and trade reference data")
	flag.Parse()

	if err := run(*league, *verbose, *cacheDir, flag.Arg(0)); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run(league string, verbose bool, cacheDir, file string) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	text, err := readInput(file)
	if err != nil {
		return err
	}
	ua := "poe-pricecheck/" + version + " (github.com/Mi7teR/poe-pricecheck)"
	hc := &http.Client{Timeout: 15 * time.Second}
	get := func(name, url string, maxAge time.Duration) (io.ReadCloser, error) {
		return cached(ctx, hc, ua, filepath.Join(cacheDir, name), url, maxAge)
	}

	t0 := time.Now()
	if league == "" {
		if league, err = currentLeague(get); err != nil {
			return err
		}
	}
	nb, err := get("ninja-"+league+".json", "https://poe.ninja/poe1/api/economy/current/dense/overviews?language=en&league="+url.QueryEscape(league), 30*time.Minute)
	if err != nil {
		return err
	}
	idx, err := ninja.Parse(nb)
	nb.Close()
	if err != nil {
		return err
	}
	sb, err := get("trade-stats.json", "https://www.pathofexile.com/api/trade/data/stats", 24*time.Hour)
	if err != nil {
		return err
	}
	stats, err := trade.ParseStats(sb)
	sb.Close()
	if err != nil {
		return err
	}
	cur, err := currencies(get)
	if err != nil {
		return err
	}
	gd, err := gamedata.PoE1()
	if err != nil {
		return err
	}
	startup := time.Since(t0)

	p := &pricer.Pricer{
		League:     league,
		Ninja:      idx,
		Trade:      &trade.Client{HTTP: hc, UserAgent: ua, POESESSID: os.Getenv("POESESSID")},
		Stats:      stats,
		Currencies: cur,
		Parser:     itemtext.New(gd),
		Fixers:     fixers.Default,
		RollSlack:  0.15,
		Now:        time.Now,
	}
	t1 := time.Now()
	r, err := p.Price(ctx, text)
	if err != nil {
		return err
	}
	took := time.Since(t1)

	divine, _ := idx.Chaos("Divine Orb")
	fmt.Println(line(r, divine))
	if verbose {
		details(r, divine, league, startup, took)
	}
	return nil
}

func line(r *pricer.Result, divine float64) string {
	if r.Source == "poe.ninja" {
		s := "~" + money(r.Chaos, divine) + " · poe.ninja"
		if r.Note != "" {
			s += " (" + r.Note + ")"
		}
		return s
	}
	if len(r.Kept) == 0 {
		return fmt.Sprintf("мало данных · %d лотов на сайте · %s", r.Total, r.Link)
	}
	fix := 0
	for _, d := range r.Dropped {
		if d.Reason == fixers.ReasonCheapOld {
			fix++
		}
	}
	return fmt.Sprintf("~%s · %d лотов, %d отброшены как фиксеры", moneyRange(r.Low, r.Mid, divine), len(r.Kept), fix)
}

func details(r *pricer.Result, divine float64, league string, startup, took time.Duration) {
	fmt.Printf("\nлига: %s · всего на сайте: %d\n", league, r.Total)
	if r.Note != "" {
		fmt.Println(r.Note)
	}
	for _, l := range r.Kept {
		fmt.Printf("  %-10s %-28s %s назад\n", money(l.Chaos, divine), l.Account, ago(l.Indexed))
	}
	for _, d := range r.Dropped {
		fmt.Printf("  ✕ %-8s %-28s %s назад — %s\n", money(d.Lot.Chaos, divine), d.Lot.Account, ago(d.Lot.Indexed), d.Reason)
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

func money(chaos, divine float64) string {
	if divine > 0 && chaos >= divine {
		return fmt.Sprintf("%.1f div", chaos/divine)
	}
	return fmt.Sprintf("%.0fc", chaos)
}

func moneyRange(lo, hi, divine float64) string {
	a, b := money(lo, divine), money(hi, divine)
	if a == b {
		return a
	}
	return a + "–" + b
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

func currentLeague(get func(string, string, time.Duration) (io.ReadCloser, error)) (string, error) {
	rc, err := get("trade-leagues.json", "https://www.pathofexile.com/api/trade/data/leagues", 6*time.Hour)
	if err != nil {
		return "", err
	}
	defer rc.Close()
	var d struct {
		Result []struct{ ID, Realm string } `json:"result"`
	}
	if err := json.NewDecoder(rc).Decode(&d); err != nil {
		return "", err
	}
	for _, l := range d.Result {
		if l.Realm != "pc" || l.ID == "Standard" {
			continue
		}
		low := strings.ToLower(l.ID)
		if strings.Contains(low, "hardcore") || strings.HasPrefix(low, "hc ") || strings.Contains(low, "ruthless") || strings.Contains(low, "ssf") {
			continue
		}
		return l.ID, nil
	}
	return "", fmt.Errorf("no temporary softcore league in the trade league list")
}

func currencies(get func(string, string, time.Duration) (io.ReadCloser, error)) (map[string]string, error) {
	rc, err := get("trade-static.json", "https://www.pathofexile.com/api/trade/data/static", 24*time.Hour)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	var d struct {
		Result []struct {
			ID      string                      `json:"id"`
			Entries []struct{ ID, Text string } `json:"entries"`
		} `json:"result"`
	}
	if err := json.NewDecoder(rc).Decode(&d); err != nil {
		return nil, err
	}
	m := map[string]string{}
	for _, g := range d.Result {
		if g.ID != "Currency" {
			continue
		}
		for _, e := range g.Entries {
			m[e.ID] = e.Text
		}
	}
	return m, nil
}

// cached returns the file at path when younger than maxAge, otherwise downloads url into it.
func cached(ctx context.Context, hc *http.Client, ua, path, src string, maxAge time.Duration) (io.ReadCloser, error) {
	if st, err := os.Stat(path); err == nil && time.Since(st.ModTime()) < maxAge {
		return os.Open(path)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", ua)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: %s", src, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return nil, err
	}
	if _, err := io.Copy(f, resp.Body); err != nil {
		f.Close()
		return nil, err
	}
	if err := f.Close(); err != nil {
		return nil, err
	}
	if err := os.Rename(tmp, path); err != nil {
		return nil, err
	}
	return os.Open(path)
}

func defaultCache() string {
	d, err := os.UserCacheDir()
	if err != nil {
		return ".cache"
	}
	return filepath.Join(d, "poe-pricecheck")
}
