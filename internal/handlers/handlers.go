// Package handlers serves ShopKeeper's HTML pages and static assets.
// All data on these pages is loaded client-side from the JSON API in
// internal/api; these handlers only render the page shell.
package handlers

import (
	"bytes"
	"encoding/json"
	"html/template"
	"io/fs"
	"net/http"

	"shop/internal/i18n"
	"shop/web"
)

// page describes one route: which content template to pair with the
// shared layout, which nav item to highlight, and which page-specific
// script (if any) to load.
type page struct {
	file       string
	active     string
	pageScript string
}

var pages = map[string]page{
	"dashboard": {file: "templates/index.html", active: "dashboard", pageScript: "dashboard.js"},
	"products":  {file: "templates/products.html", active: "products", pageScript: "products.js"},
	"sell":      {file: "templates/sell.html", active: "sell", pageScript: "sell.js"},
	"sales":     {file: "templates/sales.html", active: "sales", pageScript: "sales.js"},
	"stock-log": {file: "templates/stock-log.html", active: "stock-log", pageScript: "stock-log.js"},
	"billiard":  {file: "templates/billiard.html", active: "billiard", pageScript: "billiard.js"},
}

// langCookie remembers the language picked in the UI.
const langCookie = "lang"

// langOption is one entry of the language switcher.
type langOption struct {
	Code, Name string
	Selected   bool
}

// Handler serves pages; it holds the pre-parsed templates so a broken
// template fails fast at startup instead of on first request.
type Handler struct {
	catalog     *i18n.Catalog
	defaultLang string
	// templates and dicts are per language: every language gets its own parsed
	// copy of each page, with that language's "t" function bound in.
	templates map[string]map[string]*template.Template
	dicts     map[string]template.JS
}

// New parses each page's content template together with layout.html, once per
// language. They're parsed separately per page (not all at once) because every
// page defines a template named "content", and parsing them together would
// make later definitions silently win over earlier ones. defaultLang is used
// for visitors who haven't picked one.
func New(defaultLang string) (*Handler, error) {
	catalog, err := i18n.Load()
	if err != nil {
		return nil, err
	}
	if !i18n.Valid(defaultLang) {
		defaultLang = i18n.Default
	}
	h := &Handler{
		catalog:     catalog,
		defaultLang: defaultLang,
		templates:   map[string]map[string]*template.Template{},
		dicts:       map[string]template.JS{},
	}
	for _, lang := range i18n.Langs {
		lang := lang
		funcs := template.FuncMap{"t": func(key string) string { return catalog.T(lang, key) }}
		parsed := make(map[string]*template.Template, len(pages))
		for name, p := range pages {
			t, err := template.New("").Funcs(funcs).ParseFS(web.Templates, "templates/layout.html", p.file)
			if err != nil {
				return nil, err
			}
			parsed[name] = t
		}
		h.templates[lang] = parsed

		// json.Marshal escapes <, > and & so the table is safe inside <script>.
		raw, err := json.Marshal(catalog.Table(lang))
		if err != nil {
			return nil, err
		}
		h.dicts[lang] = template.JS(raw)
	}
	return h, nil
}

// langFor picks the request's language: the cookie if it names a supported
// one, else the configured default.
func (h *Handler) langFor(r *http.Request) string {
	if c, err := r.Cookie(langCookie); err == nil && i18n.Valid(c.Value) {
		return c.Value
	}
	return h.defaultLang
}

func (h *Handler) render(w http.ResponseWriter, r *http.Request, name string) {
	p := pages[name]
	lang := h.langFor(r)
	options := make([]langOption, 0, len(i18n.Langs))
	for _, code := range i18n.Langs {
		options = append(options, langOption{Code: code, Name: i18n.Names[code], Selected: code == lang})
	}

	// Render to a buffer first so a template error becomes a clean 500, not a
	// half-sent page.
	var buf bytes.Buffer
	data := map[string]any{
		"Active":     p.active,
		"PageScript": p.pageScript,
		"Lang":       lang,
		"LocaleTag":  i18n.LocaleTags[lang],
		"Langs":      options,
		"I18N":       h.dicts[lang],
	}
	if err := h.templates[lang][name].ExecuteTemplate(&buf, "layout", data); err != nil {
		http.Error(w, "internal error rendering page", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = buf.WriteTo(w)
}

// Routes builds the page + static-asset routing table.
func (h *Handler) Routes() (http.Handler, error) {
	mux := http.NewServeMux()

	// "GET /" alone is a subtree match covering every path; "{$}" pins
	// it to the exact root so unknown paths still 404.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { h.render(w, r, "dashboard") })
	mux.HandleFunc("GET /products", func(w http.ResponseWriter, r *http.Request) { h.render(w, r, "products") })
	mux.HandleFunc("GET /sell", func(w http.ResponseWriter, r *http.Request) { h.render(w, r, "sell") })
	mux.HandleFunc("GET /sales", func(w http.ResponseWriter, r *http.Request) { h.render(w, r, "sales") })
	mux.HandleFunc("GET /stock-log", func(w http.ResponseWriter, r *http.Request) { h.render(w, r, "stock-log") })
	mux.HandleFunc("GET /billiard", func(w http.ResponseWriter, r *http.Request) { h.render(w, r, "billiard") })

	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		return nil, err
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	// Browsers ask for /favicon.ico on their own, whatever the page links.
	mux.HandleFunc("GET /favicon.ico", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFileFS(w, r, staticFS, "favicon.ico")
	})

	return mux, nil
}
