package api

import (
	"net/http"
	"strconv"
	"time"
)

type chartEntryDTO struct {
	Date            string `json:"date"`
	Revenue         int64  `json:"revenue"`
	BilliardRevenue int64  `json:"billiard_revenue"`
	Units           int64  `json:"units"`
	SaleCount       int64  `json:"sale_count"`
}

// handleSalesChart reports the last 7 or 30 local calendar days of
// non-voided shop sales and billiard revenue, oldest first, ending
// with today. Days with nothing come back as zero entries, never
// omitted. revenue stays shop-only; billiard_revenue is reported next
// to it so the UI can show either separately or stacked.
func (a *API) handleSalesChart(w http.ResponseWriter, r *http.Request) {
	days := 7
	if v := r.URL.Query().Get("days"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || (n != 7 && n != 30) {
			writeError(w, http.StatusBadRequest, "days must be 7 or 30")
			return
		}
		days = n
	}

	now := time.Now()
	buckets, err := a.store.SalesChart(days, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load sales chart")
		return
	}
	billiardBuckets, err := a.store.BilliardChart(days, now)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "could not load sales chart")
		return
	}

	dtos := make([]chartEntryDTO, len(buckets))
	for i, b := range buckets {
		dtos[i] = chartEntryDTO{Date: b.Date, Revenue: b.Revenue, Units: b.Units, SaleCount: b.SaleCount}
		if i < len(billiardBuckets) {
			dtos[i].BilliardRevenue = billiardBuckets[i].Revenue
		}
	}
	writeJSON(w, http.StatusOK, dtos)
}
