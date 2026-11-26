package notify

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
)

const testExt = "abcdefghijklmnopabcdefghijklmnop"

func post(t *testing.T, h http.Handler, method, origin, body string) int {
	t.Helper()
	r := httptest.NewRequest(method, "/notify", strings.NewReader(body))
	if origin != "" {
		r.Header.Set("Origin", origin)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code
}

func TestHandler(t *testing.T) {
	var got []string
	var gotTab int
	var gotFrom uint16
	h := Handler(testExt, nil, func(s Search, tab int, from uint16) { got, gotTab, gotFrom = append(got, s.String()), tab, from })
	body := `{"label":"Tabula Rasa Simple Robe","url":"https://www.pathofexile.com/trade/search/Covens/Ab3d/live","tab":417}`

	if c := post(t, h, "POST", "https://evil.example", body); c != http.StatusForbidden {
		t.Errorf("foreign origin: %d, want 403", c)
	}
	if c := post(t, h, "POST", "", body); c != http.StatusForbidden {
		t.Errorf("no origin: %d, want 403", c)
	}
	if c := post(t, h, "GET", "chrome-extension://"+testExt, ""); c != http.StatusMethodNotAllowed {
		t.Errorf("GET: %d, want 405", c)
	}
	if c := post(t, h, "POST", "chrome-extension://"+testExt, "{"); c != http.StatusBadRequest {
		t.Errorf("bad json: %d, want 400", c)
	}
	if len(got) != 0 {
		t.Fatalf("rejected requests notified: %q", got)
	}
	if c := post(t, h, "POST", "chrome-extension://"+testExt, body); c != http.StatusNoContent {
		t.Errorf("good request: %d, want 204", c)
	}
	if len(got) != 1 || got[0] != "Tabula Rasa Simple Robe" || gotTab != 417 {
		t.Errorf("notified %q, tab %d", got, gotTab)
	}
	if gotFrom != 1234 { // httptest's RemoteAddr is 192.0.2.1:1234
		t.Errorf("from port %d, want 1234", gotFrom)
	}
}

func TestDescribe(t *testing.T) {
	// the user's live search, saved from the site on 2026-11-26
	long := "https://www.pathofexile.com/trade/search/Covens/H4sIAAAAAAAAEzXMOwrAIBBF0a2EV7uCWYZJFyxGYkDwRxwLEfceDKS9B-5AFZZWQQO5iM8JBE4dUyFxdCAcbFvgTXNlKEgvK-4-luA2na2D-h4VdI6fOV1QuH0Q9yww08wXdPJclWsAAAA/live"
	for _, c := range []struct{ label, url, want string }{
		{"Tabula Rasa Simple Robe", long, "Tabula Rasa Simple Robe"},
		{"  ", long, "Tabula Rasa Simple Robe"},
		{"", "https://www.pathofexile.com/trade/search/Covens/Ab3dEfGh/live", "live Ab3dEfGh"},
		{"", "garbage", "live search"},
	} {
		if got := Describe(c.label, c.url, nil).String(); got != c.want {
			t.Errorf("Describe(%q, %q) = %q, want %q", c.label, c.url, got, c.want)
		}
	}
}

// pack builds a long search URL the way the trade site does.
func pack(t *testing.T, query string) string {
	t.Helper()
	var b bytes.Buffer
	zw := gzip.NewWriter(&b)
	zw.Write([]byte(query))
	zw.Close()
	return "https://www.pathofexile.com/trade/search/Covens/" + base64.RawURLEncoding.EncodeToString(b.Bytes()) + "/live"
}

func TestDescribeFilters(t *testing.T) {
	texts := map[string]string{
		"pseudo.pseudo_adds_elemental_damage_to_spells": "Adds # to # Elemental Damage to Spells",
		"pseudo.pseudo_increased_spell_damage":          "#% increased Spell Damage",
		"pseudo.pseudo_total_elemental_resistance":      "+#% total Elemental Resistance",
		"pseudo.pseudo_total_life":                      "+# total maximum Life",
		"explicit.stat_1":                               "#% increased Attack Speed",
	}
	stat := func(id string) (string, bool) { s, ok := texts[id]; return s, ok }
	// the user's wand search, 2026-11-26: its only stat group is switched off
	wand := "https://www.pathofexile.com/trade/search/Covens/H4sIAAAAAAAAE22QwQrCQAxE_2XOi-jBy_6KyBKbKAvb3aVJlVL679JWpGJzSTKBF2ZGqJH1Cj-iVIslw4PygMktF4W_jLChyqIzHO4xmXTrITI8qkrP5bC2QMwaJEkr2SgFppYeEqwErZKSwuFJqZf5YRsz_Ok4OXBUuiVh-DsllcntomNuOiEVXlkf9h_xvEe8biXr-kX5Wlkths2-GRsyeZRu-MnoJVRLPrzmTKa53g3A6nNKAQAA/live"
	on := pack(t, `{"stats":[{"type":"and","filters":[{"id":"pseudo.pseudo_adds_elemental_damage_to_spells","value":{"min":10}},{"id":"pseudo.pseudo_increased_spell_damage","value":{"min":50,"max":80}},{"id":"explicit.stat_1","disabled":true}]}],"filters":{"type_filters":{"filters":{"category":{"option":"weapon.wand"}}}}}`)
	many := pack(t, `{"type":"Leather Belt","stats":[{"type":"and","filters":[{"id":"pseudo.pseudo_total_life","value":{"min":70}},{"id":"pseudo.pseudo_total_elemental_resistance","value":{"min":80}},{"id":"pseudo.pseudo_increased_spell_damage"},{"id":"unknown.stat"},{"id":"explicit.stat_1","value":{"max":5}}]},{"type":"not","filters":[{"id":"explicit.stat_1"}]}]}`)
	for _, c := range []struct{ field, url, want string }{
		{"", wand, "wand"},
		{"", on, "wand · Adds # to # Elemental Damage to Spells (10+), 50–80% increased Spell Damage"},
		{"", many, "Leather Belt · 70+ total maximum Life, 80+% total Elemental Resistance, #% increased Spell Damage, +2"},
		{"Mageblood", on, "Mageblood · Adds # to # Elemental Damage to Spells (10+), 50–80% increased Spell Damage"},
		{"", pack(t, `{"status":{"option":"any"}}`), "any item"},
	} {
		if got := Describe(c.field, c.url, stat).String(); got != c.want {
			t.Errorf("Describe(%q, %.60q)\n got %q\nwant %q", c.field, c.url, got, c.want)
		}
	}
	if got := Describe("", on, nil).String(); got != "wand" {
		t.Errorf("no stat texts: %q, want wand", got)
	}
}

func TestExtensionIDMatchesManifest(t *testing.T) {
	b, err := os.ReadFile("../../extension/manifest.json")
	if err != nil {
		t.Fatal(err)
	}
	var m struct{ Key string }
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatal(err)
	}
	der, err := base64.StdEncoding.DecodeString(m.Key)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(der)
	id := []byte(hex.EncodeToString(sum[:16]))
	for i, c := range id { // Chrome writes the hex digits as a–p
		if c <= '9' {
			id[i] = 'a' + c - '0'
		} else {
			id[i] = 'a' + 10 + c - 'a'
		}
	}
	if string(id) != ExtensionID {
		t.Errorf("manifest key gives ID %s, ExtensionID is %s", id, ExtensionID)
	}
}

func TestFocusURL(t *testing.T) {
	if got := FocusURL(417); got != "http://127.0.0.1:47317/focus?tab=417" {
		t.Errorf("FocusURL = %q", got)
	}
}
