package pricer

import (
	"context"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Mi7teR/poe-pricecheck/internal/trade"
)

// recordedTrade answers trade search and fetch with the recorded boots
// responses, counts the requests and keeps the search bodies.
type recordedTrade struct {
	n        atomic.Int32
	mu       sync.Mutex
	searches []string
}

func (rt *recordedTrade) RoundTrip(r *http.Request) (*http.Response, error) {
	rt.n.Add(1)
	if r.Body != nil {
		b, _ := io.ReadAll(r.Body)
		rt.mu.Lock()
		rt.searches = append(rt.searches, string(b))
		rt.mu.Unlock()
	}
	file := "search-boots.json"
	if strings.Contains(r.URL.Path, "/fetch/") {
		file = "fetch-boots.json"
	}
	b, err := os.ReadFile("../../testdata/recorded/" + file)
	if err != nil {
		return nil, err
	}
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": {"application/json"}},
		Body: io.NopCloser(strings.NewReader(string(b))), Request: r}, nil
}

func TestTradeCache(t *testing.T) {
	p := offline(t)
	rt := &recordedTrade{}
	now := time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)
	p.Trade = &trade.Client{HTTP: &http.Client{Transport: rt}}
	p.Stats, p.League, p.Now, p.RollSlack = recordedStats(t), "Covens", func() time.Time { return now }, 0.15
	text := item(t, "boots_rare.txt")

	first, err := p.Price(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	sent := rt.n.Load()
	if sent == 0 {
		t.Fatal("first check sent no trade request")
	}
	now = now.Add(14 * time.Minute)
	again, err := p.Price(context.Background(), text)
	if err != nil {
		t.Fatal(err)
	}
	if rt.n.Load() != sent {
		t.Errorf("same search within 15 min sent %d more requests", rt.n.Load()-sent)
	}
	if again.Line(200) != first.Line(200) {
		t.Errorf("cached line %q, first %q", again.Line(200), first.Line(200))
	}
	now = now.Add(2 * time.Minute) // 16 min after the first search
	if _, err := p.Price(context.Background(), text); err != nil {
		t.Fatal(err)
	}
	if rt.n.Load() == sent {
		t.Error("search older than 15 min was served from the cache")
	}
}
