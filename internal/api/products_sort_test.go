package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestListProductsSortParam(t *testing.T) {
	h := newTestAPI(t).Routes(8080)
	for sort, want := range map[string]int{"": 200, "name": 200, "popular": 200, "bogus": 400} {
		req := httptest.NewRequest(http.MethodGet, "/api/products?sort="+sort, nil)
		req.Host = "127.0.0.1:8080"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("sort=%q: status %d, want %d", sort, rec.Code, want)
		}
	}
}
