package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"shop/internal/store"
)

type productDTO struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	SKU               string `json:"sku"`
	Price             int64  `json:"price"`
	Stock             int64  `json:"stock"`
	LowStockThreshold int64  `json:"low_stock_threshold"`
	Archived          bool   `json:"archived"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
}

func toProductDTO(p store.Product) productDTO {
	return productDTO{
		ID: p.ID, Name: p.Name, SKU: p.SKU, Price: p.Price, Stock: p.Stock,
		LowStockThreshold: p.LowStockThreshold, Archived: p.Archived,
		CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt,
	}
}

func (a *API) handleListProducts(w http.ResponseWriter, r *http.Request) {
	q := r.URL.Query().Get("q")

	var archived *bool
	if v := r.URL.Query().Get("archived"); v != "" {
		b := v == "1"
		if v != "0" && v != "1" {
			writeError(w, http.StatusBadRequest, "archived must be 0 or 1")
			return
		}
		archived = &b
	}

	products, err := a.store.ListProducts(q, archived)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not list products")
		return
	}

	dtos := make([]productDTO, 0, len(products))
	for _, p := range products {
		dtos = append(dtos, toProductDTO(p))
	}
	writeJSON(w, http.StatusOK, dtos)
}

type createProductRequest struct {
	Name              string `json:"name"`
	SKU               string `json:"sku"`
	Price             int64  `json:"price"`
	Stock             int64  `json:"stock"`
	LowStockThreshold *int64 `json:"low_stock_threshold"`
}

func (a *API) handleCreateProduct(w http.ResponseWriter, r *http.Request) {
	var req createProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	name, err := validateName(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sku, err := validateSKU(req.SKU)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	threshold := int64(5)
	if req.LowStockThreshold != nil {
		threshold = *req.LowStockThreshold
	}
	if err := validatePrice(req.Price); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateStock(req.Stock); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateThreshold(threshold); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	product, err := a.store.CreateProduct(name, sku, req.Price, req.Stock, threshold)
	if errors.Is(err, store.ErrDuplicateSKU) {
		writeError(w, http.StatusConflict, "sku already in use")
		return
	} else if err != nil {
		writeError(w, http.StatusInternalServerError, "could not create product")
		return
	}
	writeJSON(w, http.StatusCreated, toProductDTO(product))
}

type updateProductRequest struct {
	Name              string `json:"name"`
	SKU               string `json:"sku"`
	Price             int64  `json:"price"`
	LowStockThreshold int64  `json:"low_stock_threshold"`
}

func (a *API) handleUpdateProduct(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	var req updateProductRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}

	name, err := validateName(req.Name)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	sku, err := validateSKU(req.SKU)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validatePrice(req.Price); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if err := validateThreshold(req.LowStockThreshold); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	product, err := a.store.UpdateProduct(id, name, sku, req.Price, req.LowStockThreshold)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "product not found")
	case errors.Is(err, store.ErrDuplicateSKU):
		writeError(w, http.StatusConflict, "sku already in use")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not update product")
	default:
		writeJSON(w, http.StatusOK, toProductDTO(product))
	}
}

type adjustRequest struct {
	Delta  int64  `json:"delta"`
	Reason string `json:"reason"`
	Note   string `json:"note"`
}

func (a *API) handleAdjustStock(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r)
	if err != nil {
		writeError(w, http.StatusBadRequest, "invalid product id")
		return
	}

	var req adjustRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON body")
		return
	}
	if req.Delta == 0 {
		writeError(w, http.StatusBadRequest, "delta must not be zero")
		return
	}
	reason, err := validateAdjustReason(req.Reason)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	note, err := validateNote(req.Note)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	newStock, err := a.store.AdjustStock(id, req.Delta, reason, note)
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeError(w, http.StatusNotFound, "product not found")
	case errors.Is(err, store.ErrInsufficientStock):
		writeError(w, http.StatusConflict, "adjustment would make stock negative")
	case err != nil:
		writeError(w, http.StatusInternalServerError, "could not adjust stock")
	default:
		writeJSON(w, http.StatusOK, map[string]int64{"stock": newStock})
	}
}

func (a *API) handleSetArchived(archived bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id, err := parseID(r)
		if err != nil {
			writeError(w, http.StatusBadRequest, "invalid product id")
			return
		}
		err = a.store.SetArchived(id, archived)
		switch {
		case errors.Is(err, store.ErrNotFound):
			writeError(w, http.StatusNotFound, "product not found")
		case err != nil:
			writeError(w, http.StatusInternalServerError, "could not update product")
		default:
			writeJSON(w, http.StatusOK, map[string]bool{"archived": archived})
		}
	}
}

func parseID(r *http.Request) (int64, error) {
	return strconv.ParseInt(r.PathValue("id"), 10, 64)
}
