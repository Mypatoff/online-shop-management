package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListSalesLimitAboveMaxIsCapped(t *testing.T) {
	a := newTestAPI(t)
	p, err := a.store.CreateProduct("Cola", "", 150, 600, 5)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	for i := 0; i < maxSalesLimit+1; i++ {
		mustSell(t, a, p.ID, 1)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sales?limit=100000", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusOK, rec.Body.String())
	}
	var body []saleDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body) != maxSalesLimit {
		t.Fatalf("len(body) = %d, want %d (capped)", len(body), maxSalesLimit)
	}
}
