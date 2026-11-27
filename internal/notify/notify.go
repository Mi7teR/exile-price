// Package notify receives "a live search tab sounded" from the browser
// extension in extension/. The extension only reads the tab: its URL, that
// it plays a sound, and the text of the search field. Nothing here talks to
// the trade site.
package notify

import (
	"bytes"
	"compress/gzip"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
)

// Port is where the extension posts; extension/sw.js has the same number.
const Port = 47317

// Handler accepts POST {"label","url","tab"} from the extension with the
// given ID only, and calls on with the search (see Describe), the browser
// tab and the local port the browser posted from (to find which browser it
// is; 0 if unknown). on runs while the browser's connection is still open.
func Handler(extID string, stat StatText, on func(s Search, tab int, from uint16)) http.Handler {
	origin := "chrome-extension://" + extID
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != origin { // a web page cannot fake this
			http.Error(w, "forbidden", http.StatusForbidden)
			return
		}
		if r.Method != http.MethodPost {
			http.Error(w, "POST only", http.StatusMethodNotAllowed)
			return
		}
		var m struct {
			Label, URL string
			Tab        int
		}
		if err := json.NewDecoder(io.LimitReader(r.Body, 8<<10)).Decode(&m); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		var from uint16
		if ap, err := netip.ParseAddrPort(r.RemoteAddr); err == nil {
			from = ap.Port()
		}
		on(Describe(m.Label, m.URL, stat), m.Tab, from)
		w.WriteHeader(http.StatusNoContent)
	})
}

// FocusURL is what a click on the notice opens in the default browser. The
// extension sees it, closes it and brings that tab to the front; the page
// itself (FocusPage) only shows a line if the extension is missing.
func FocusURL(tab int) string {
	return fmt.Sprintf("http://127.0.0.1:%d/focus?tab=%d", Port, tab)
}

// FocusPage answers FocusURL.
func FocusPage(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	io.WriteString(w, `<!doctype html><title>exile-price</title><p>exile-price: the browser extension did not pick this up. Is it installed in this browser?</p>`)
}

// StatText gives the trade site's text for a stat id, with # for the
// numbers (trade.Stats.Text). It may be nil: then no filters are shown.
type StatText func(id string) (string, bool)

// Search is what a notice says about a live search.
type Search struct {
	Name    string
	Filters []string // enabled stat filters as text, in query order
}

// MaxFilters is how many filters a notice lists before "+N".
const MaxFilters = 3

// String is the search on one line, for the log.
func (s Search) String() string {
	f := s.Filters
	if len(f) == 0 {
		return s.Name
	}
	if len(f) > MaxFilters {
		f = append(f[:MaxFilters:MaxFilters], fmt.Sprintf("+%d", len(f)-MaxFilters))
	}
	return s.Name + " · " + strings.Join(f, ", ")
}

// Describe names the search and lists its filters. The name is the search
// field's text, else the item name and type packed in a long search URL
// (gzip + base64url of the query), else its category, else the search id.
// Filters come from the same packed query; disabled ones are skipped.
func Describe(field, url string, stat StatText) Search {
	parts := strings.Split(strings.TrimSuffix(url, "/live"), "/")
	id := parts[len(parts)-1]
	q, decoded := decodeQuery(id)
	name := strings.TrimSpace(field)
	if name == "Any" {
		name = ""
	}
	if name == "" {
		name = strings.TrimSpace(q.Name + " " + q.Type)
	}
	if name == "" && q.Filters.TypeFilters.Filters.Category != nil {
		c := q.Filters.TypeFilters.Filters.Category.Option
		name = c[strings.LastIndex(c, ".")+1:]
	}
	switch {
	case name != "":
	case decoded:
		name = "any item"
	case strings.Contains(url, "/trade/search/"):
		name = "live " + id
	default:
		name = "live search"
	}
	return Search{Name: name, Filters: filters(q, stat)}
}

type query struct {
	Name, Type string
	Stats      []struct {
		Type     string
		Disabled bool
		Filters  []struct {
			ID       string
			Disabled bool
			Value    *struct{ Min, Max *float64 }
		}
	}
	Filters struct {
		TypeFilters struct {
			Filters struct {
				Category *struct{ Option string }
			}
		} `json:"type_filters"`
	}
}

// filters lists the enabled stat filters as text: "50+% increased Spell
// Damage", "Adds # to # Elemental Damage to Spells (10+)".
func filters(q query, stat StatText) []string {
	if stat == nil {
		return nil
	}
	var out []string
	for _, g := range q.Stats {
		if g.Disabled {
			continue
		}
		for _, f := range g.Filters {
			if f.Disabled {
				continue
			}
			t, ok := stat(f.ID)
			if !ok {
				continue
			}
			if f.Value != nil {
				t = withValue(t, f.Value.Min, f.Value.Max)
			}
			if g.Type == "not" {
				t = "not " + t
			}
			out = append(out, t)
		}
	}
	return out
}

// withValue puts the filter's range into the stat text in place of its one
// #, or after the text when it has several.
func withValue(text string, lo, hi *float64) string {
	n := func(v *float64) string { return strconv.FormatFloat(*v, 'f', -1, 64) }
	var v string
	switch {
	case lo != nil && hi != nil:
		v = n(lo) + "–" + n(hi)
	case lo != nil:
		v = n(lo) + "+"
	case hi != nil:
		v = "≤" + n(hi)
	default:
		return text
	}
	if strings.Count(text, "#") != 1 {
		return text + " (" + v + ")"
	}
	return strings.Replace(strings.Replace(text, "+#", "#", 1), "#", v, 1)
}

func decodeQuery(id string) (q query, ok bool) {
	if !strings.HasPrefix(id, "H4sI") { // gzip magic, base64
		return q, false
	}
	b, err := base64.RawURLEncoding.DecodeString(strings.TrimRight(id, "="))
	if err != nil {
		return q, false
	}
	zr, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		return q, false
	}
	if err := json.NewDecoder(io.LimitReader(zr, 64<<10)).Decode(&q); err != nil {
		return q, false
	}
	return q, true
}
