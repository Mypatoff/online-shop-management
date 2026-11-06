package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func doJSON(t *testing.T, a *API, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = "127.0.0.1:8080"
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(rec, req)
	return rec
}

func TestCupPriceEndpoints(t *testing.T) {
	a := newTestAPI(t)

	var got map[string]int64
	rec := doJSON(t, a, "GET", "/api/cup-price", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != 200 || got["price"] != 0 {
		t.Fatalf("GET before set = %d %s", rec.Code, rec.Body)
	}
	if rec := doJSON(t, a, "PUT", "/api/cup-price", `{"price":500}`); rec.Code != 200 {
		t.Fatalf("PUT = %d %s", rec.Code, rec.Body)
	}
	rec = doJSON(t, a, "GET", "/api/cup-price", "")
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || got["price"] != 500 {
		t.Fatalf("GET after set = %s", rec.Body)
	}
	for _, bad := range []string{`{"price":-1}`, `{"price":100000000001}`, `{"price":"x"}`, `nope`} {
		if rec := doJSON(t, a, "PUT", "/api/cup-price", bad); rec.Code != http.StatusBadRequest {
			t.Errorf("PUT %s = %d, want 400", bad, rec.Code)
		}
	}
	if rec := doJSON(t, a, "GET", "/api/cup-price", ""); !bytes.Contains(rec.Body.Bytes(), []byte("500")) {
		t.Errorf("a rejected PUT changed the price: %s", rec.Body)
	}
}

func TestCreateSaleWithCups(t *testing.T) {
	a := newTestAPI(t)
	_, err := a.store.CreateProduct("Cola", "", 150, 10, 5)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetCupPrice(20); err != nil {
		t.Fatal(err)
	}

	rec := doJSON(t, a, "POST", "/api/sales", `{"product_id":1,"quantity":2,"cups":3}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var out struct {
		Sale  saleDTO `json:"sale"`
		Stock int64   `json:"stock"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if out.Sale.Cups != 3 || out.Sale.CupPrice != 20 || out.Sale.Total != 360 || out.Stock != 8 {
		t.Fatalf("response = %+v, want 3 cups at 20, total 360, stock 8", out)
	}

	// "cups" is optional: omitting it is a sale with none.
	rec = doJSON(t, a, "POST", "/api/sales", `{"product_id":1,"quantity":1}`)
	if rec.Code != http.StatusCreated || !strings.Contains(rec.Body.String(), `"cups":0`) {
		t.Fatalf("sale without cups: %d %s", rec.Code, rec.Body)
	}

	for _, bad := range []string{`"cups":-1`, `"cups":1001`} {
		rec := doJSON(t, a, "POST", "/api/sales", `{"product_id":1,"quantity":1,`+bad+`}`)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400", bad, rec.Code)
		}
	}
	// A refused request sold nothing.
	if sales, _ := a.store.ListSales(10); len(sales) != 2 {
		t.Errorf("%d sales recorded, want only the 2 good ones", len(sales))
	}
}

func TestDailyReportKeepsCupMoneyOutOfTheProduct(t *testing.T) {
	a := newTestAPI(t)
	if _, err := a.store.CreateProduct("Cola", "", 150, 10, 5); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetCupPrice(20); err != nil {
		t.Fatal(err)
	}
	if _, _, err := a.store.CreateSale(1, 2, 3); err != nil {
		t.Fatal(err)
	}

	res, body := doDailyRequest(t, a, "")
	if res.StatusCode != 200 {
		t.Fatalf("status %d", res.StatusCode)
	}
	if body.Totals.Revenue != 360 || body.Totals.TotalRevenue != 360 {
		t.Errorf("totals = %+v, want revenue 360 including the cups", body.Totals)
	}
	if len(body.ByProduct) != 1 || body.ByProduct[0].Revenue != 300 || body.ByProduct[0].Units != 2 {
		t.Errorf("by_product = %+v, want Cola with revenue 300 (cups not credited to it)", body.ByProduct)
	}
	if body.Cups.Count != 3 || body.Cups.Revenue != 60 {
		t.Errorf("cups = %+v, want 3 cups bringing in 60", body.Cups)
	}
	if body.Totals.Units != 2 {
		t.Errorf("units = %d, want 2: cups are not product units", body.Totals.Units)
	}
}

func TestDailyReportDoesNotCountVoidedCups(t *testing.T) {
	a := newTestAPI(t)
	if _, err := a.store.CreateProduct("Cola", "", 150, 10, 5); err != nil {
		t.Fatal(err)
	}
	if err := a.store.SetCupPrice(20); err != nil {
		t.Fatal(err)
	}
	sale, _, err := a.store.CreateSale(1, 1, 4)
	if err != nil {
		t.Fatal(err)
	}
	if err := a.store.VoidSale(sale.ID); err != nil {
		t.Fatal(err)
	}
	_, body := doDailyRequest(t, a, "")
	if body.Cups.Count != 0 || body.Cups.Revenue != 0 || body.Totals.Revenue != 0 {
		t.Errorf("a voided sale's cups were counted: cups %+v, totals %+v", body.Cups, body.Totals)
	}
}
