// Package trade talks to the pathofexile.com trade API: search, fetch and
// rate limits read only from the X-Rate-Limit-* response headers.
package trade

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const base = "https://www.pathofexile.com/api/trade"

// FetchLimit is the most result ids one fetch request accepts; 11+ returns 400.
const FetchLimit = 10

type Client struct {
	HTTP      *http.Client
	UserAgent string
	POESESSID string // optional

	mu      sync.Mutex
	blocked map[string]time.Time // policy → not before
}

// RateLimitError says how long to wait before the policy allows another request.
type RateLimitError struct {
	Policy string
	Wait   time.Duration
}

func (e *RateLimitError) Error() string {
	return fmt.Sprintf("trade: rate limit %s, wait %s", e.Policy, e.Wait.Round(time.Second))
}

type SearchResult struct {
	ID     string   `json:"id"`
	Total  int      `json:"total"`
	Result []string `json:"result"`
}

type Listing struct {
	Indexed time.Time `json:"indexed"`
	Price   *struct {
		Type     string  `json:"type"`
		Amount   float64 `json:"amount"`
		Currency string  `json:"currency"`
	} `json:"price"`
	Account struct {
		Name   string `json:"name"`
		Online *struct {
			Status string `json:"status"` // "afk" or empty
		} `json:"online"`
	} `json:"account"`
}

type FetchItem struct {
	ID      string  `json:"id"`
	Listing Listing `json:"listing"`
	Item    struct {
		Name     string `json:"name"`
		TypeLine string `json:"typeLine"`
		// Mods are objects with tier and roll ranges, not plain strings.
		ExplicitMods []Mod `json:"explicitMods"`
		ImplicitMods []Mod `json:"implicitMods"`
	} `json:"item"`
}

type Mod struct {
	Description string `json:"description"`
	Hash        string `json:"hash"` // "stat.explicit.stat_3299347043"
}

// Search posts a query and returns the result ids (up to 100, sorted as asked).
func (c *Client) Search(ctx context.Context, league string, q Query) (*SearchResult, error) {
	body, err := json.Marshal(q)
	if err != nil {
		return nil, err
	}
	var out SearchResult
	err = c.do(ctx, "search", http.MethodPost, base+"/search/"+url.PathEscape(league), body, &out)
	return &out, err
}

// Fetch returns listings for at most FetchLimit ids.
func (c *Client) Fetch(ctx context.Context, queryID string, ids []string) ([]FetchItem, error) {
	if len(ids) > FetchLimit {
		return nil, fmt.Errorf("trade: fetch takes at most %d ids, got %d", FetchLimit, len(ids))
	}
	var out struct {
		Result []FetchItem `json:"result"`
	}
	u := base + "/fetch/" + strings.Join(ids, ",") + "?query=" + url.QueryEscape(queryID)
	err := c.do(ctx, "fetch", http.MethodGet, u, nil, &out)
	return out.Result, err
}

// SearchURL is the trade site page for a query, for "open in browser".
func SearchURL(league, queryID string) string {
	return "https://www.pathofexile.com/trade/search/" + url.PathEscape(league) + "/" + queryID
}

func (c *Client) do(ctx context.Context, policy, method, u string, body []byte, out any) error {
	if wait := c.waitFor(policy); wait > 0 {
		// Wait out a short block if the caller's deadline allows it;
		// otherwise fail now rather than sleep into a timeout.
		if dl, ok := ctx.Deadline(); !ok || time.Until(dl) < wait {
			return &RateLimitError{Policy: policy, Wait: wait}
		}
		t := time.NewTimer(wait)
		select {
		case <-ctx.Done():
			t.Stop()
			return ctx.Err()
		case <-t.C:
		}
	}
	req, err := http.NewRequestWithContext(ctx, method, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", c.UserAgent)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.POESESSID != "" {
		req.AddCookie(&http.Cookie{Name: "POESESSID", Value: c.POESESSID})
	}
	resp, err := c.HTTP.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	c.track(policy, resp)
	if resp.StatusCode == http.StatusTooManyRequests {
		return &RateLimitError{Policy: policy, Wait: retryAfter(resp.Header)}
	}
	if resp.StatusCode != http.StatusOK {
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.NewDecoder(resp.Body).Decode(&e)
		return fmt.Errorf("trade %s: %s: %s", policy, resp.Status, e.Error.Message)
	}
	return json.NewDecoder(resp.Body).Decode(out)
}

func (c *Client) waitFor(policy string) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if t, ok := c.blocked[policy]; ok {
		if d := time.Until(t); d > 0 {
			return d
		}
	}
	return 0
}

// track blocks the policy until the tightest full window resets when the
// next request would exceed it. Rules look like "5:10:60,15:60:300"
// (hits:period:penalty), state like "1:10:0,1:60:0" (hits:period:active penalty).
func (c *Client) track(policy string, resp *http.Response) {
	var wait time.Duration
	for _, rule := range strings.Split(resp.Header.Get("X-Rate-Limit-Rules"), ",") {
		rule = strings.TrimSpace(rule)
		if rule == "" {
			continue
		}
		limits := parseTriples(resp.Header.Get("X-Rate-Limit-" + rule))
		states := parseTriples(resp.Header.Get("X-Rate-Limit-" + rule + "-State"))
		for i := 0; i < len(limits) && i < len(states); i++ {
			lim, st := limits[i], states[i]
			if st[2] > 0 { // active penalty
				wait = max(wait, time.Duration(st[2])*time.Second)
			} else if st[0] >= lim[0] { // window full
				wait = max(wait, time.Duration(lim[1])*time.Second)
			}
		}
	}
	if resp.StatusCode == http.StatusTooManyRequests {
		wait = max(wait, retryAfter(resp.Header))
	}
	if wait > 0 {
		c.mu.Lock()
		if c.blocked == nil {
			c.blocked = map[string]time.Time{}
		}
		c.blocked[policy] = time.Now().Add(wait)
		c.mu.Unlock()
	}
}

func retryAfter(h http.Header) time.Duration {
	s, _ := strconv.Atoi(h.Get("Retry-After"))
	return time.Duration(s) * time.Second
}

func parseTriples(s string) [][3]int {
	var out [][3]int
	for _, part := range strings.Split(s, ",") {
		f := strings.Split(strings.TrimSpace(part), ":")
		if len(f) != 3 {
			continue
		}
		var t [3]int
		for i := range f {
			t[i], _ = strconv.Atoi(f[i])
		}
		out = append(out, t)
	}
	return out
}
