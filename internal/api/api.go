// Package api implements ShopKeeper's HTTP routes: a small JSON API,
// protected by middleware since the app runs with no login.
package api

import (
	"net/http"

	"shop/internal/store"
)

// API holds everything the HTTP handlers need.
type API struct {
	store    *store.Store
	shopName string
	currency string
	decimals int
}

func New(s *store.Store, shopName, currency string, decimals int) *API {
	return &API{store: s, shopName: shopName, currency: currency, decimals: decimals}
}

// Routes builds the full handler: the routing table wrapped in the
// Content-Type and Host/Origin protection middleware.
func (a *API) Routes(port int) http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/config", a.handleConfig)
	mux.HandleFunc("GET /api/summary", a.handleSummary)
	mux.HandleFunc("GET /api/backup", a.handleBackup)
	mux.HandleFunc("GET /api/stock-log", a.handleStockLog)

	mux.HandleFunc("GET /api/products", a.handleListProducts)
	mux.HandleFunc("POST /api/products", a.handleCreateProduct)
	mux.HandleFunc("PUT /api/products/{id}", a.handleUpdateProduct)
	mux.HandleFunc("POST /api/products/{id}/adjust", a.handleAdjustStock)
	mux.HandleFunc("POST /api/products/{id}/archive", a.handleSetArchived(true))
	mux.HandleFunc("POST /api/products/{id}/unarchive", a.handleSetArchived(false))

	mux.HandleFunc("GET /api/sales", a.handleListSales)
	mux.HandleFunc("POST /api/sales", a.handleCreateSale)
	mux.HandleFunc("POST /api/sales/{id}/void", a.handleVoidSale)
	mux.HandleFunc("GET /api/sales/daily", a.handleDailySales)
	mux.HandleFunc("GET /api/sales/chart", a.handleSalesChart)

	mux.HandleFunc("GET /api/billiard", a.handleListBilliard)
	mux.HandleFunc("POST /api/billiard", a.handleCreateBilliardEntry)
	mux.HandleFunc("POST /api/billiard/{id}/void", a.handleVoidBilliardEntry)

	return protect(allowedHosts(port))(requireJSON(mux))
}
