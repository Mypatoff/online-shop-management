// Package handlers serves ShopKeeper's HTML pages and static assets.
// All data on these pages is loaded client-side from the JSON API in
// internal/api; these handlers only render the page shell.
package handlers

import (
	"html/template"
	"io/fs"
	"net/http"

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

// Handler serves pages; it holds the pre-parsed templates so a broken
// template fails fast at startup instead of on first request.
type Handler struct {
	templates map[string]*template.Template
}

// New parses each page's content template together with layout.html.
// They're parsed separately per page (not all at once) because every
// page defines a template named "content", and parsing them together
// would make later definitions silently win over earlier ones.
func New() (*Handler, error) {
	parsed := make(map[string]*template.Template, len(pages))
	for name, p := range pages {
		t, err := template.ParseFS(web.Templates, "templates/layout.html", p.file)
		if err != nil {
			return nil, err
		}
		parsed[name] = t
	}
	return &Handler{templates: parsed}, nil
}

func (h *Handler) render(w http.ResponseWriter, name string) {
	p := pages[name]
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	data := map[string]any{"Active": p.active, "PageScript": p.pageScript}
	if err := h.templates[name].ExecuteTemplate(w, "layout", data); err != nil {
		http.Error(w, "internal error rendering page", http.StatusInternalServerError)
	}
}

// Routes builds the page + static-asset routing table.
func (h *Handler) Routes() (http.Handler, error) {
	mux := http.NewServeMux()

	// "GET /" alone is a subtree match covering every path; "{$}" pins
	// it to the exact root so unknown paths still 404.
	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) { h.render(w, "dashboard") })
	mux.HandleFunc("GET /products", func(w http.ResponseWriter, r *http.Request) { h.render(w, "products") })
	mux.HandleFunc("GET /sell", func(w http.ResponseWriter, r *http.Request) { h.render(w, "sell") })
	mux.HandleFunc("GET /sales", func(w http.ResponseWriter, r *http.Request) { h.render(w, "sales") })
	mux.HandleFunc("GET /stock-log", func(w http.ResponseWriter, r *http.Request) { h.render(w, "stock-log") })
	mux.HandleFunc("GET /billiard", func(w http.ResponseWriter, r *http.Request) { h.render(w, "billiard") })

	staticFS, err := fs.Sub(web.Static, "static")
	if err != nil {
		return nil, err
	}
	mux.Handle("GET /static/", http.StripPrefix("/static/", http.FileServerFS(staticFS)))

	return mux, nil
}
