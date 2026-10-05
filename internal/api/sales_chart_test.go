package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func doChartRequest(t *testing.T, a *API, query string) (*http.Response, []chartEntryDTO) {
	t.Helper()
	target := "/api/sales/chart"
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()

	a.Routes(8080).ServeHTTP(rec, req)

	res := rec.Result()
	var body []chartEntryDTO
	if res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return res, body
}

func TestSalesChartInvalidDaysIs400(t *testing.T) {
	a := newTestAPI(t)

	res, _ := doChartRequest(t, a, "days=5")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusBadRequest)
	}
}

func TestSalesChartDefaultIs7Days(t *testing.T) {
	a := newTestAPI(t)

	res, body := doChartRequest(t, a, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if len(body) != 7 {
		t.Fatalf("len(body) = %d, want 7", len(body))
	}
}

func TestSalesChart30Days(t *testing.T) {
	a := newTestAPI(t)

	res, body := doChartRequest(t, a, "days=30")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if len(body) != 30 {
		t.Fatalf("len(body) = %d, want 30", len(body))
	}
}
