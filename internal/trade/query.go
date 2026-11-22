package trade

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strconv"
	"strings"
)

type Query struct {
	Query struct {
		Status  Option        `json:"status"`
		Name    string        `json:"name,omitempty"`
		Type    string        `json:"type,omitempty"`
		Stats   []StatGroup   `json:"stats"`
		Filters *QueryFilters `json:"filters,omitempty"`
	} `json:"query"`
	Sort map[string]string `json:"sort"`
}

type Option struct {
	Option string `json:"option"`
}

type StatGroup struct {
	Type    string       `json:"type"` // "and", "count"
	Filters []StatFilter `json:"filters"`
	Value   *MinMax      `json:"value,omitempty"` // for "count"
}

type StatFilter struct {
	ID    string  `json:"id"`
	Value *MinMax `json:"value,omitempty"`
}

type MinMax struct {
	Min *float64 `json:"min,omitempty"`
	Max *float64 `json:"max,omitempty"`
}

type QueryFilters struct {
	TypeFilters *struct {
		Filters map[string]Option `json:"filters"`
	} `json:"type_filters,omitempty"`
}

// NewQuery is an online-only query sorted by price ascending.
func NewQuery() Query {
	var q Query
	q.Query.Status = Option{"online"}
	q.Query.Stats = []StatGroup{}
	q.Sort = map[string]string{"price": "asc"}
	return q
}

// SetTypeFilter sets a type_filters option such as category or rarity.
func (q *Query) SetTypeFilter(key, option string) {
	if q.Query.Filters == nil {
		q.Query.Filters = &QueryFilters{}
	}
	if q.Query.Filters.TypeFilters == nil {
		q.Query.Filters.TypeFilters = &struct {
			Filters map[string]Option `json:"filters"`
		}{Filters: map[string]Option{}}
	}
	q.Query.Filters.TypeFilters.Filters[key] = Option{option}
}

// Stats maps mod text to trade stat ids, built from /api/trade/data/stats.
type Stats struct {
	byKind map[string]map[string]string // kind ("explicit", "implicit", "pseudo") → template → id
}

// Match is a mod line resolved to a trade stat and its first number.
type Match struct {
	ID    string
	Text  string
	Value float64
}

var numRe = regexp.MustCompile(`[+-]?\d+(?:\.\d+)?`)

// ParseStats reads the /api/trade/data/stats response.
func ParseStats(r io.Reader) (*Stats, error) {
	var d struct {
		Result []struct {
			ID      string `json:"id"`
			Entries []struct {
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"entries"`
		} `json:"result"`
	}
	if err := json.NewDecoder(r).Decode(&d); err != nil {
		return nil, fmt.Errorf("trade stats: %w", err)
	}
	s := &Stats{byKind: map[string]map[string]string{}}
	for _, g := range d.Result {
		m := map[string]string{}
		for _, e := range g.Entries {
			if _, dup := m[e.Text]; !dup { // first entry wins, as on the trade site
				m[e.Text] = e.ID
			}
		}
		s.byKind[g.ID] = m
	}
	return s, nil
}

// Resolve matches one mod line of the given kind. The trade site writes
// every number as "#" and drops the leading "+" for some stats, so both
// spellings are tried.
func (s *Stats) Resolve(kind, line string) (Match, bool) {
	m := s.byKind[kind]
	nums := numRe.FindAllString(line, -1)
	tmpl := numRe.ReplaceAllStringFunc(line, func(n string) string {
		if strings.HasPrefix(n, "+") {
			return "+#"
		}
		if strings.HasPrefix(n, "-") {
			return "-#"
		}
		return "#"
	})
	for _, t := range []string{tmpl, strings.ReplaceAll(tmpl, "+#", "#")} {
		if id, ok := m[t]; ok {
			var v float64
			if len(nums) > 0 {
				v, _ = strconv.ParseFloat(strings.TrimPrefix(nums[0], "+"), 64)
			}
			return Match{ID: id, Text: line, Value: v}, true
		}
	}
	return Match{}, false
}
