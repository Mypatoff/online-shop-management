package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"shop/internal/store"
)

type saleDTO struct {
	ID          int64  `json:"id"`
	ProductID   int64  `json:"product_id"`
	ProductName string `json:"product_name"`
	Quantity    int64  `json:"quantity"`
	UnitPrice   int64  `json:"unit_price"`
	Cups        int64  `json:"cups"`
	CupPrice    int64  `json:"cup_price"`
	Total       int64  `json:"total"`
	CreatedAt   int64  `json:"created_at"`
	Voided      bool   `json:"voided"`
}

func toSaleDTO(s store.Sale) saleDTO {
	return saleDTO{
		ID: s.ID, ProductID: s.ProductID, ProductName: s.ProductName,
		Quantity: s.Quantity, UnitPrice: s.UnitPrice, Cups: s.Cups, CupPrice: s.CupPrice, Total: s.Total,
		CreatedAt: s.CreatedAt, Voided: s.Voided(),
	}
}

type createSaleRequest struct {
	ProductID int64 `json:"product_id"`
	Quantity  int64 `json:"quantity"`
	Cups      int64 `json:"cups"` // optional; charged at the current cup price
}

func (a *API) handleCreateSale(w http.ResponseWriter, r *http.Request) {
	var req createSaleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validateQuantity(req.Quantity); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateCups(req.Cups); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	sale, newStock, err := a.store.CreateSale(req.ProductID, req.Quantity, req.Cups)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "product not found")
	case errors.Is(err, store.ErrArchived):
		writeError(w, http.StatusConflict, "product is archived")
	case errors.Is(err, store.ErrInsufficientStock):
		writeError(w, http.StatusConflict, "not enough stock")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not record sale")
	default:
		writeJSON(w, http.StatusCreated, map[string]any{
			"sale":  toSaleDTO(sale),
			"stock": newStock,
		})
	}
}

func (a *API) handleVoidSale(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid sale id")
		return
	}

	err = a.store.VoidSale(id)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "sale not found")
	case errors.Is(err, store.ErrAlreadyVoided):
		writeError(w, http.StatusConflict, "sale already voided")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not void sale")
	default:
		writeJSON(w, http.StatusOK, map[string]bool{"voided": true})
	}
}

const maxSalesLimit = 500

func (a *API) handleListSales(w http.ResponseWriter, r *http.Request) {
	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeError(w, http.StatusBadRequest, "limit must be a positive integer")
			return
		}
		limit = n
	}
	if limit > maxSalesLimit {
		limit = maxSalesLimit
	}

	sales, err := a.store.ListSales(limit)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list sales")
		return
	}
	dtos := make([]saleDTO, 0, len(sales))
	for _, s := range sales {
		dtos = append(dtos, toSaleDTO(s))
	}
	writeJSON(w, http.StatusOK, dtos)
}

// handleGetCupPrice returns what one cup costs when sold with a drink (0 if unset).
func (a *API) handleGetCupPrice(w http.ResponseWriter, r *http.Request) {
	price, err := a.store.CupPrice()
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not read the cup price")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"price": price})
}

// handleSetCupPrice changes the cup price for sales made from now on.
func (a *API) handleSetCupPrice(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Price int64 `json:"price"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if err := validatePrice(req.Price); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := a.store.SetCupPrice(req.Price); err != nil {
		writeError(w, http.StatusInternalServerError, "could not save the cup price")
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"price": req.Price})
}
