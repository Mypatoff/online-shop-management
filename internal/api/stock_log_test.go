package api

import (
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestAdjustStockInvalidReasonIs400(t *testing.T) {
	a := newTestAPI(t)
	p, err := a.store.CreateProduct("Cola", "", 150, 10, 5)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/products/"+strconv.FormatInt(p.ID, 10)+"/adjust",
		strings.NewReader(`{"delta": 5, "reason": "because"}`))
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.Routes(8080).ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestAdjustStockValidReasonSucceedsAndLogs(t *testing.T) {
	a := newTestAPI(t)
	p, err := a.store.CreateProduct("Cola", "", 150, 10, 5)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, "/api/products/"+strconv.FormatInt(p.ID, 10)+"/adjust",
		strings.NewReader(`{"delta": 5, "reason": "restock", "note": "delivery"}`))
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	a.Routes(8080).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d; body = %s", rec.Code, http.StatusOK, rec.Body.String())
	}

	logReq := httptest.NewRequest(http.MethodGet, "/api/stock-log?product_id="+strconv.FormatInt(p.ID, 10), nil)
	logReq.Host = "127.0.0.1:8080"
	logRec := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(logRec, logReq)

	if logRec.Code != http.StatusOK {
		t.Fatalf("stock-log status = %d, want %d; body = %s", logRec.Code, http.StatusOK, logRec.Body.String())
	}
	if !strings.Contains(logRec.Body.String(), `"reason":"restock"`) {
		t.Fatalf("stock-log body missing adjust movement: %s", logRec.Body.String())
	}
}
