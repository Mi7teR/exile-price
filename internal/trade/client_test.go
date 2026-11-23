package trade

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestDoWaitsOutShortBlock(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := &Client{HTTP: srv.Client(), blocked: map[string]time.Time{"search": time.Now().Add(200 * time.Millisecond)}}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	t0 := time.Now()
	if err := c.do(ctx, "search", http.MethodGet, srv.URL, nil, &struct{}{}); err != nil {
		t.Fatalf("block shorter than the deadline: %v", err)
	}
	if took := time.Since(t0); took < 150*time.Millisecond {
		t.Errorf("did not wait: %v", took)
	}
}

func TestDoFailsWhenBlockOutlastsDeadline(t *testing.T) {
	c := &Client{HTTP: http.DefaultClient, blocked: map[string]time.Time{"search": time.Now().Add(time.Minute)}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	t0 := time.Now()
	err := c.do(ctx, "search", http.MethodGet, "http://127.0.0.1:1", nil, &struct{}{})
	var rl *RateLimitError
	if !errors.As(err, &rl) {
		t.Fatalf("want RateLimitError, got %v", err)
	}
	if time.Since(t0) > 100*time.Millisecond {
		t.Errorf("slept before failing")
	}
}

func TestFetchItemOnlineShapes(t *testing.T) {
	for _, online := range []string{`false`, `null`, `{}`, `{"status":"afk"}`} {
		var it FetchItem
		if err := json.Unmarshal([]byte(`{"listing":{"account":{"name":"a","online":`+online+`}}}`), &it); err != nil {
			t.Errorf("online=%s: %v", online, err)
		}
	}
}
