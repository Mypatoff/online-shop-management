package api

import (
	"net/http"
	"strconv"

	"shop/internal/store"
)

type stockMovementDTO struct {
	ID          int64  `json:"id"`
	ProductID   int64  `json:"product_id"`
	ProductName string `json:"product_name"`
	Type        string `json:"type"`
	Delta       int64  `json:"delta"`
	StockAfter  int64  `json:"stock_after"`
	Reason      string `json:"reason"`
	Note        string `json:"note"`
	SaleID      *int64 `json:"sale_id,omitempty"`
	CreatedAt   int64  `json:"created_at"`
}

func toStockMovementDTO(m store.StockMovement) stockMovementDTO {
	return stockMovementDTO{
		ID: m.ID, ProductID: m.ProductID, ProductName: m.ProductName,
		Type: m.Type, Delta: m.Delta, StockAfter: m.StockAfter,
		Reason: m.Reason, Note: m.Note, SaleID: m.SaleID, CreatedAt: m.CreatedAt,
	}
}

const (
	defaultStockLogLimit = 100
	maxStockLogLimit     = 500
)

// handleStockLog serves GET /api/stock-log?product_id=&limit=&before_id=,
// newest first. next_before_id is set when the page came back full,
// a signal there may be more to page through.
func (a *API) handleStockLog(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query()

	var productID *int64
	if v := q.Get("product_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "product_id must be an integer")
			return
		}
		productID = &id
	}

	limit := defaultStockLogLimit
	if v := q.Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = n
	}
	if limit > maxStockLogLimit {
		limit = maxStockLogLimit
	}

	var beforeID *int64
	if v := q.Get("before_id"); v != "" {
		id, err := strconv.ParseInt(v, 10, 64)
		if err != nil {
			writeError(w, http.StatusBadRequest, "before_id must be an integer")
			return
		}
		beforeID = &id
	}

	movements, err := a.store.ListStockMovements(productID, limit, beforeID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list stock log")
		return
	}

	dtos := make([]stockMovementDTO, 0, len(movements))
	for _, m := range movements {
		dtos = append(dtos, toStockMovementDTO(m))
	}

	var nextBeforeID *int64
	if len(movements) == limit {
		id := movements[len(movements)-1].ID
		nextBeforeID = &id
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"movements":      dtos,
		"next_before_id": nextBeforeID,
	})
}
