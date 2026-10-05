package api

import (
	"net/http"
	"sort"
	"time"
)

type dailyTotalsDTO struct {
	Revenue      int64 `json:"revenue"`
	Units        int64 `json:"units"`
	SaleCount    int64 `json:"sale_count"`
	VoidedCount  int64 `json:"voided_count"`
	TotalRevenue int64 `json:"total_revenue"`
}

type dailyProductDTO struct {
	Name    string `json:"name"`
	Units   int64  `json:"units"`
	Revenue int64  `json:"revenue"`
}

type dailySalesDTO struct {
	Date      string               `json:"date"`
	Sales     []saleDTO            `json:"sales"`
	Totals    dailyTotalsDTO       `json:"totals"`
	ByProduct []dailyProductDTO    `json:"by_product"`
	Billiard  billiardDailyPartDTO `json:"billiard"`
}

type billiardDailyPartDTO struct {
	Entries []billiardEntryDTO `json:"entries"`
	Revenue int64              `json:"revenue"`
	Count   int64              `json:"count"`
}

const dailyDateLayout = "2006-01-02"

// handleDailySales reports one local calendar day's sales: the raw
// list (oldest first, voided included), totals over the non-voided
// ones, and a per-product revenue breakdown.
func (a *API) handleDailySales(w http.ResponseWriter, r *http.Request) {
	day := time.Now()
	if v := r.URL.Query().Get("date"); v != "" {
		parsed, err := time.ParseInLocation(dailyDateLayout, v, time.Local)
		if err != nil {
			writeError(w, http.StatusBadRequest, "date must be in YYYY-MM-DD format")
			return
		}
		day = parsed
	}

	sales, err := a.store.DailySales(day)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load daily sales")
		return
	}
	billiardEntries, err := a.store.DailyBilliard(day)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load daily billiard entries")
		return
	}

	resp := dailySalesDTO{
		Date:      day.Format(dailyDateLayout),
		Sales:     make([]saleDTO, 0, len(sales)),
		ByProduct: []dailyProductDTO{},
		Billiard:  billiardDailyPartDTO{Entries: make([]billiardEntryDTO, 0, len(billiardEntries))},
	}

	for _, e := range billiardEntries {
		resp.Billiard.Entries = append(resp.Billiard.Entries, toBilliardEntryDTO(e))
		if !e.Voided() {
			resp.Billiard.Revenue += e.Amount
			resp.Billiard.Count++
		}
	}

	byProduct := make(map[string]*dailyProductDTO)
	var order []string
	for _, sale := range sales {
		resp.Sales = append(resp.Sales, toSaleDTO(sale))
		if sale.Voided() {
			resp.Totals.VoidedCount++
			continue
		}
		resp.Totals.Revenue += sale.Total
		resp.Totals.Units += sale.Quantity
		resp.Totals.SaleCount++

		entry, ok := byProduct[sale.ProductName]
		if !ok {
			entry = &dailyProductDTO{Name: sale.ProductName}
			byProduct[sale.ProductName] = entry
			order = append(order, sale.ProductName)
		}
		entry.Units += sale.Quantity
		entry.Revenue += sale.Total
	}

	for _, name := range order {
		resp.ByProduct = append(resp.ByProduct, *byProduct[name])
	}
	sort.SliceStable(resp.ByProduct, func(i, j int) bool {
		return resp.ByProduct[i].Revenue > resp.ByProduct[j].Revenue
	})

	resp.Totals.TotalRevenue = resp.Totals.Revenue + resp.Billiard.Revenue

	writeJSON(w, http.StatusOK, resp)
}
