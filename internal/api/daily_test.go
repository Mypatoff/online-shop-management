package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func doDailyRequest(t *testing.T, a *API, query string) (*http.Response, dailySalesDTO) {
	t.Helper()
	target := "/api/sales/daily"
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()

	a.Routes(8080).ServeHTTP(rec, req)

	res := rec.Result()
	var body dailySalesDTO
	if res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return res, body
}

func mustSell(t *testing.T, a *API, productID, quantity int64) {
	t.Helper()
	if _, _, err := a.store.CreateSale(productID, quantity); err != nil {
		t.Fatalf("create sale: %v", err)
	}
}

func TestDailySalesInvalidDateIs400(t *testing.T) {
	a := newTestAPI(t)

	res, _ := doDailyRequest(t, a, "date=not-a-date")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusBadRequest)
	}
}

func TestDailySalesEmptyDayIsZeroNotNull(t *testing.T) {
	a := newTestAPI(t)

	req := httptest.NewRequest(http.MethodGet, "/api/sales/daily?date=2026-01-01", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	raw := rec.Body.String()
	if !jsonContains(raw, `"sales":[]`) {
		t.Fatalf("response sales field is not an empty array: %s", raw)
	}
	if !jsonContains(raw, `"by_product":[]`) {
		t.Fatalf("response by_product field is not an empty array: %s", raw)
	}

	var body dailySalesDTO
	if err := json.Unmarshal([]byte(raw), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body.Totals.Revenue != 0 || body.Totals.Units != 0 || body.Totals.SaleCount != 0 || body.Totals.VoidedCount != 0 {
		t.Fatalf("totals = %+v, want all zero", body.Totals)
	}
}

func jsonContains(raw, needle string) bool {
	for i := 0; i+len(needle) <= len(raw); i++ {
		if raw[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}

func TestDailySalesVoidedExcludedFromTotalsButListed(t *testing.T) {
	a := newTestAPI(t)
	p, err := a.store.CreateProduct("Cola", "", 150, 100, 5)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	mustSell(t, a, p.ID, 2)
	sale, _, err := a.store.CreateSale(p.ID, 3)
	if err != nil {
		t.Fatalf("create sale: %v", err)
	}
	if err := a.store.VoidSale(sale.ID); err != nil {
		t.Fatalf("void sale: %v", err)
	}

	res, body := doDailyRequest(t, a, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}

	if len(body.Sales) != 2 {
		t.Fatalf("len(sales) = %d, want 2 (voided sale must still be listed)", len(body.Sales))
	}
	if body.Totals.SaleCount != 1 || body.Totals.Units != 2 || body.Totals.Revenue != 300 {
		t.Fatalf("totals = %+v, want only the non-voided sale counted", body.Totals)
	}
	if body.Totals.VoidedCount != 1 {
		t.Fatalf("voided_count = %d, want 1", body.Totals.VoidedCount)
	}
}

func TestDailySalesByProductSumsMatchTotalsAndSortedByRevenue(t *testing.T) {
	a := newTestAPI(t)
	cola, err := a.store.CreateProduct("Cola", "", 150, 100, 5)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	chips, err := a.store.CreateProduct("Chips", "", 500, 100, 5)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	mustSell(t, a, cola.ID, 4)  // 600
	mustSell(t, a, chips.ID, 1) // 500

	_, body := doDailyRequest(t, a, "")

	if len(body.ByProduct) != 2 {
		t.Fatalf("len(by_product) = %d, want 2", len(body.ByProduct))
	}
	if body.ByProduct[0].Name != "Cola" || body.ByProduct[0].Revenue != 600 {
		t.Fatalf("by_product[0] = %+v, want Cola with revenue 600 (sorted desc)", body.ByProduct[0])
	}
	if body.ByProduct[1].Name != "Chips" || body.ByProduct[1].Revenue != 500 {
		t.Fatalf("by_product[1] = %+v, want Chips with revenue 500", body.ByProduct[1])
	}

	var sumRevenue, sumUnits int64
	for _, item := range body.ByProduct {
		sumRevenue += item.Revenue
		sumUnits += item.Units
	}
	if sumRevenue != body.Totals.Revenue {
		t.Fatalf("by_product revenue sum = %d, totals.revenue = %d", sumRevenue, body.Totals.Revenue)
	}
	if sumUnits != body.Totals.Units {
		t.Fatalf("by_product units sum = %d, totals.units = %d", sumUnits, body.Totals.Units)
	}
}
