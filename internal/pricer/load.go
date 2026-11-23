package pricer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Mi7teR/poe-pricecheck/internal/i18n"

	"github.com/Mi7teR/poe-lib/gamedata"
	"github.com/Mi7teR/poe-lib/itemtext"
	"github.com/Mi7teR/poe-pricecheck/internal/config"
	"github.com/Mi7teR/poe-pricecheck/internal/fixers"
	"github.com/Mi7teR/poe-pricecheck/internal/ninja"
	"github.com/Mi7teR/poe-pricecheck/internal/trade"
)

const (
	tradeData = "https://www.pathofexile.com/api/trade/data/"
	ninjaURL  = "https://poe.ninja/poe1/api/economy/current/dense/overviews?language=en&league="
)

type Options struct {
	HTTP      *http.Client
	UserAgent string
	CacheDir  string
	POESESSID string
	Config    config.Config
}

// Load fetches (or reads from the cache) everything a price check needs.
// The league list is always fetched: it picks the league and opens the
// keep-alive connection to pathofexile.com that searches reuse.
func Load(ctx context.Context, o Options) (*Pricer, error) {
	l := loader{o: o, ctx: ctx}
	league := o.Config.League
	var leagues struct {
		Result []struct{ ID, Realm string } `json:"result"`
	}
	if err := l.json("trade-leagues.json", tradeData+"leagues", 0, &leagues); err != nil {
		return nil, err
	}
	var all []string
	for _, lg := range leagues.Result {
		if lg.Realm == "pc" {
			all = append(all, lg.ID)
		}
	}
	if league == "" {
		league = AutoLeague(all)
		if league == "" {
			return nil, errors.New(i18n.T("the trade league list has no temporary softcore league; set league in config.toml"))
		}
	}

	var stats *trade.Stats
	if err := l.with("trade-stats.json", tradeData+"stats", 24*time.Hour, func(r io.Reader) (err error) {
		stats, err = trade.ParseStats(r)
		return err
	}); err != nil {
		return nil, err
	}

	var static struct {
		Result []struct {
			ID      string                      `json:"id"`
			Entries []struct{ ID, Text string } `json:"entries"`
		} `json:"result"`
	}
	if err := l.json("trade-static.json", tradeData+"static", 24*time.Hour, &static); err != nil {
		return nil, err
	}
	cur := map[string]string{}
	for _, g := range static.Result {
		if g.ID == "Currency" {
			for _, e := range g.Entries {
				cur[e.ID] = e.Text
			}
		}
	}

	gd, err := gamedata.PoE1()
	if err != nil {
		return nil, err
	}
	c := o.Config
	p := &Pricer{
		League:     league,
		Leagues:    all,
		Trade:      &trade.Client{HTTP: o.HTTP, UserAgent: o.UserAgent, POESESSID: o.POESESSID},
		Stats:      stats,
		Currencies: cur,
		Parser:     itemtext.New(gd),
		Fixers:     fixers.Config{CheapRatio: c.Fixers.CheapRatio, StaleAge: c.Fixers.StaleAge, MinReference: c.Fixers.MinReferenceChaos},
		RollSlack:  c.RollSlack,
		Now:        time.Now,
	}
	if err := p.loadNinja(ctx, o, 30*time.Minute); err != nil {
		return nil, err
	}
	return p, nil
}

// RefreshNinja downloads fresh poe.ninja prices and swaps them in.
func (p *Pricer) RefreshNinja(ctx context.Context, o Options) error {
	return p.loadNinja(ctx, o, 0)
}

func (p *Pricer) loadNinja(ctx context.Context, o Options, maxAge time.Duration) error {
	l := loader{o: o, ctx: ctx}
	return l.with("ninja-"+p.League+".json", ninjaURL+url.QueryEscape(p.League), maxAge, func(r io.Reader) error {
		idx, err := ninja.Parse(r)
		if err == nil {
			p.SetNinja(idx)
		}
		return err
	})
}

// Warm keeps the connection to pathofexile.com open between price checks:
// the league list is served from Cloudflare's cache and has no rate limit.
func Warm(ctx context.Context, o Options) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, tradeData+"leagues", nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", o.UserAgent)
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return err
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return resp.Body.Close()
}

// AutoLeague is the first temporary softcore league in GGG's order, or "".
func AutoLeague(ids []string) string {
	for _, id := range ids {
		if isTemporarySoftcore(id) {
			return id
		}
	}
	return ""
}

func isTemporarySoftcore(id string) bool {
	low := strings.ToLower(id)
	return id != "Standard" && !strings.Contains(low, "hardcore") && !strings.HasPrefix(low, "hc ") &&
		!strings.Contains(low, "ruthless") && !strings.Contains(low, "ssf")
}

type loader struct {
	o   Options
	ctx context.Context
}

func (l loader) json(name, src string, maxAge time.Duration, v any) error {
	return l.with(name, src, maxAge, func(r io.Reader) error { return json.NewDecoder(r).Decode(v) })
}

// with hands f the cached file when younger than maxAge, else downloads src
// into the cache first. A failed download falls back to a stale cached copy.
func (l loader) with(name, src string, maxAge time.Duration, f func(io.Reader) error) error {
	path := filepath.Join(l.o.CacheDir, name)
	st, statErr := os.Stat(path)
	if statErr != nil || maxAge == 0 || time.Since(st.ModTime()) >= maxAge {
		if err := l.download(src, path); err != nil && statErr != nil {
			return err
		}
	}
	fh, err := os.Open(path)
	if err != nil {
		return err
	}
	defer fh.Close()
	if err := f(fh); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	return nil
}

func (l loader) download(src, path string) error {
	req, err := http.NewRequestWithContext(l.ctx, http.MethodGet, src, nil)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", l.o.UserAgent)
	resp, err := l.o.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GET %s: %s", src, resp.Status)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	fh, err := os.Create(tmp)
	if err != nil {
		return err
	}
	if _, err := io.Copy(fh, resp.Body); err != nil {
		fh.Close()
		return err
	}
	if err := fh.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
