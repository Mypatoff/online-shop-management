package handlers

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"strings"
	"testing"

	"shop/internal/i18n"
)

func newRoutes(t *testing.T, lang string) http.Handler {
	t.Helper()
	h, err := New(lang)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	routes, err := h.Routes()
	if err != nil {
		t.Fatalf("Routes: %v", err)
	}
	return routes
}

func get(h http.Handler, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodGet, path, nil)
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

var pagePaths = []string{"/", "/products", "/sell", "/sales", "/stock-log", "/billiard"}

func TestEveryPageRendersInEveryLanguage(t *testing.T) {
	for _, lang := range i18n.Langs {
		routes := newRoutes(t, lang)
		for _, path := range pagePaths {
			rec := get(routes, path)
			if rec.Code != http.StatusOK {
				t.Errorf("%s [%s]: status %d", path, lang, rec.Code)
				continue
			}
			body := rec.Body.String()
			if ct := rec.Header().Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
				t.Errorf("%s: content type %q", path, ct)
			}
			for _, want := range []string{
				`<html lang="` + lang + `">`,
				`/static/app.js`, `/static/maintenance.js`, `/static/style.css`,
				`rel="icon"`, `id="lang-select"`, `id="modal"`,
			} {
				if !strings.Contains(body, want) {
					t.Errorf("%s [%s]: page lacks %q", path, lang, want)
				}
			}
			// A key shown instead of its text means a template asked for a
			// translation that does not exist.
			if m := regexp.MustCompile(`>\s*[a-z]+\.[a-z_0-9]+\s*<`).FindString(body); m != "" {
				t.Errorf("%s [%s]: untranslated key in page: %s", path, lang, m)
			}
		}
	}
}

func TestPageUsesTheChosenLanguage(t *testing.T) {
	routes := newRoutes(t, "en")
	c := mustCatalog(t)

	if body := get(routes, "/sell").Body.String(); !strings.Contains(body, c.T("en", "sell.title")) {
		t.Error("default language not English")
	}
	for _, lang := range []string{"uz", "ru"} {
		body := get(routes, "/sell", &http.Cookie{Name: "lang", Value: lang}).Body.String()
		if !strings.Contains(body, c.T(lang, "sell.title")) || !strings.Contains(body, `<html lang="`+lang+`">`) {
			t.Errorf("cookie lang=%s was not honoured", lang)
		}
		if !strings.Contains(body, "window.I18N") || !strings.Contains(body, c.T(lang, "sell.sold")[:6]) {
			t.Errorf("the %s dictionary was not embedded for the scripts", lang)
		}
	}
	// A configured default applies without a cookie; a bad cookie is ignored.
	if body := get(newRoutes(t, "ru"), "/sell").Body.String(); !strings.Contains(body, c.T("ru", "sell.title")) {
		t.Error("configured default language ignored")
	}
	if rec := get(routes, "/sell", &http.Cookie{Name: "lang", Value: `x"><script>`}); rec.Code != 200 ||
		strings.Contains(rec.Body.String(), `<script>"`) || !strings.Contains(rec.Body.String(), `<html lang="en">`) {
		t.Error("an invalid language cookie must fall back to the default")
	}
	// An unsupported default falls back to English rather than failing.
	if rec := get(newRoutes(t, "de"), "/"); rec.Code != 200 || !strings.Contains(rec.Body.String(), `<html lang="en">`) {
		t.Error("unsupported default language should fall back to English")
	}
}

func mustCatalog(t *testing.T) *i18n.Catalog {
	t.Helper()
	c, err := i18n.Load()
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestEmbeddedDictionaryIsSafeInsideScriptTag(t *testing.T) {
	// "</script>" or "<!--" in a translation must not be able to end the script.
	for _, lang := range i18n.Langs {
		body := get(newRoutes(t, lang), "/").Body.String()
		start := strings.Index(body, "window.I18N")
		end := strings.Index(body[start:], "</script>")
		if start < 0 || end < 0 {
			t.Fatalf("[%s] no embedded dictionary found", lang)
		}
		if strings.Contains(body[start:start+end], "<") {
			t.Errorf("[%s] raw '<' inside the embedded dictionary", lang)
		}
	}
}

func TestStaticAssetsAreServed(t *testing.T) {
	routes := newRoutes(t, "en")
	for path, wantType := range map[string]string{
		"/static/style.css":      "text/css",
		"/static/app.js":         "javascript",
		"/static/maintenance.js": "javascript",
		"/static/update.js":      "javascript",
		"/static/dashboard.js":   "javascript",
		"/static/products.js":    "javascript",
		"/static/sell.js":        "javascript",
		"/static/sales.js":       "javascript",
		"/static/stock-log.js":   "javascript",
		"/static/billiard.js":    "javascript",
	} {
		rec := get(routes, path)
		if rec.Code != 200 || rec.Body.Len() == 0 {
			t.Errorf("%s: status %d, %d bytes", path, rec.Code, rec.Body.Len())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, wantType) {
			t.Errorf("%s: content type %q, want %q", path, ct, wantType)
		}
	}
	for path, wantType := range map[string]string{
		"/static/icon.svg": "svg", "/static/favicon.ico": "icon", "/favicon.ico": "icon",
	} {
		rec := get(routes, path)
		if rec.Code != 200 || rec.Body.Len() == 0 {
			t.Errorf("%s: status %d, %d bytes", path, rec.Code, rec.Body.Len())
		}
		if ct := rec.Header().Get("Content-Type"); !strings.Contains(ct, wantType) {
			t.Errorf("%s: content type %q, want it to contain %q", path, ct, wantType)
		}
	}
	if rec := get(routes, "/static/nope.js"); rec.Code != http.StatusNotFound {
		t.Errorf("missing asset: status %d, want 404", rec.Code)
	}
}

func TestEveryScriptAPageLoadsExists(t *testing.T) {
	routes := newRoutes(t, "en")
	for _, path := range pagePaths {
		body := get(routes, path).Body.String()
		for _, m := range regexp.MustCompile(`(?:src|href)="(/static/[^"]+)"`).FindAllStringSubmatch(body, -1) {
			if rec := get(routes, m[1]); rec.Code != 200 {
				t.Errorf("%s links %s, which answers %d", path, m[1], rec.Code)
			}
		}
	}
}

func TestUnknownPathIs404AndOnlyGETIsServed(t *testing.T) {
	routes := newRoutes(t, "en")
	if rec := get(routes, "/no-such-page"); rec.Code != http.StatusNotFound {
		t.Errorf("unknown page: %d, want 404", rec.Code)
	}
	req := httptest.NewRequest(http.MethodPost, "/products", nil)
	rec := httptest.NewRecorder()
	routes.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Errorf("POST /products: %d, want 405", rec.Code)
	}
}
