package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func doBilliardPost(t *testing.T, a *API, body string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/billiard", strings.NewReader(body))
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(rec, req)
	return rec
}

func doBilliardGet(t *testing.T, a *API, query string) (*http.Response, billiardDailyDTO) {
	t.Helper()
	target := "/api/billiard"
	if query != "" {
		target += "?" + query
	}
	req := httptest.NewRequest(http.MethodGet, target, nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(rec, req)

	res := rec.Result()
	var body billiardDailyDTO
	if res.StatusCode == http.StatusOK {
		if err := json.NewDecoder(res.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
	return res, body
}

func TestCreateBilliardEntryValid(t *testing.T) {
	a := newTestAPI(t)
	rec := doBilliardPost(t, a, `{"amount":1500,"table":"Table 1","minutes":45,"note":"regular"}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
	var entry billiardEntryDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if entry.Amount != 1500 || entry.Table != "Table 1" || entry.Note != "regular" {
		t.Fatalf("entry = %+v, want amount 1500 table 'Table 1' note regular", entry)
	}
	if entry.Minutes == nil || *entry.Minutes != 45 {
		t.Fatalf("minutes = %v, want 45", entry.Minutes)
	}
	if entry.Voided {
		t.Fatal("new entry should not be voided")
	}
}

func TestCreateBilliardEntryAmountZeroIs400(t *testing.T) {
	a := newTestAPI(t)
	rec := doBilliardPost(t, a, `{"amount":0}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCreateBilliardEntryAmountNegativeIs400(t *testing.T) {
	a := newTestAPI(t)
	rec := doBilliardPost(t, a, `{"amount":-500}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCreateBilliardEntryAmountTooLargeIs400(t *testing.T) {
	a := newTestAPI(t)
	rec := doBilliardPost(t, a, `{"amount":100000000001}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCreateBilliardEntryMinutesOutOfRangeIs400(t *testing.T) {
	a := newTestAPI(t)
	rec := doBilliardPost(t, a, `{"amount":1000,"minutes":1441}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCreateBilliardEntryTableTooLongIs400(t *testing.T) {
	a := newTestAPI(t)
	longTable := ""
	for i := 0; i < 31; i++ {
		longTable += "a"
	}
	rec := doBilliardPost(t, a, `{"amount":1000,"table":"`+longTable+`"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusBadRequest, rec.Body.String())
	}
}

func TestCreateBilliardEntryOmittedMinutesIsValid(t *testing.T) {
	a := newTestAPI(t)
	rec := doBilliardPost(t, a, `{"amount":1000}`)
	if rec.Code != http.StatusCreated {
		t.Fatalf("status = %d, want %d, body: %s", rec.Code, http.StatusCreated, rec.Body.String())
	}
}

func TestVoidBilliardEntryTwiceIs409(t *testing.T) {
	a := newTestAPI(t)
	rec := doBilliardPost(t, a, `{"amount":1000}`)
	var entry billiardEntryDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &entry); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	voidReq := httptest.NewRequest(http.MethodPost, "/api/billiard/"+strconv.FormatInt(entry.ID, 10)+"/void", strings.NewReader("{}"))
	voidReq.Host = "127.0.0.1:8080"
	voidReq.Header.Set("Content-Type", "application/json")
	voidRec := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(voidRec, voidReq)
	if voidRec.Code != http.StatusOK {
		t.Fatalf("first void status = %d, want %d, body: %s", voidRec.Code, http.StatusOK, voidRec.Body.String())
	}

	voidReq2 := httptest.NewRequest(http.MethodPost, "/api/billiard/"+strconv.FormatInt(entry.ID, 10)+"/void", strings.NewReader("{}"))
	voidReq2.Host = "127.0.0.1:8080"
	voidReq2.Header.Set("Content-Type", "application/json")
	voidRec2 := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(voidRec2, voidReq2)
	if voidRec2.Code != http.StatusConflict {
		t.Fatalf("second void status = %d, want %d, body: %s", voidRec2.Code, http.StatusConflict, voidRec2.Body.String())
	}
}

func TestVoidBilliardEntryUnknownIdIs404(t *testing.T) {
	a := newTestAPI(t)

	voidReq := httptest.NewRequest(http.MethodPost, "/api/billiard/999999/void", strings.NewReader("{}"))
	voidReq.Host = "127.0.0.1:8080"
	voidReq.Header.Set("Content-Type", "application/json")
	voidRec := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(voidRec, voidReq)
	if voidRec.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d, body: %s", voidRec.Code, http.StatusNotFound, voidRec.Body.String())
	}
}

func TestListBilliardVoidedExcludedFromTotalsButListed(t *testing.T) {
	a := newTestAPI(t)

	rec1 := doBilliardPost(t, a, `{"amount":1000}`)
	var e1 billiardEntryDTO
	json.Unmarshal(rec1.Body.Bytes(), &e1) //nolint:errcheck

	rec2 := doBilliardPost(t, a, `{"amount":2000}`)
	var e2 billiardEntryDTO
	json.Unmarshal(rec2.Body.Bytes(), &e2) //nolint:errcheck

	voidReq := httptest.NewRequest(http.MethodPost, "/api/billiard/"+strconv.FormatInt(e2.ID, 10)+"/void", strings.NewReader("{}"))
	voidReq.Host = "127.0.0.1:8080"
	voidReq.Header.Set("Content-Type", "application/json")
	voidRec := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(voidRec, voidReq)
	if voidRec.Code != http.StatusOK {
		t.Fatalf("void status = %d, want %d", voidRec.Code, http.StatusOK)
	}

	res, body := doBilliardGet(t, a, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if len(body.Entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2 (voided entry must still be listed)", len(body.Entries))
	}
	if body.Totals.Revenue != 1000 || body.Totals.Count != 1 {
		t.Fatalf("totals = %+v, want only the non-voided entry counted", body.Totals)
	}
}

func TestListBilliardInvalidDateIs400(t *testing.T) {
	a := newTestAPI(t)
	res, _ := doBilliardGet(t, a, "date=not-a-date")
	if res.StatusCode != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusBadRequest)
	}
}

func TestSummaryIncludesBilliardAndTotalRevenue(t *testing.T) {
	a := newTestAPI(t)

	p, err := a.store.CreateProduct("Cola", "", 150, 100, 5)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	if _, _, err := a.store.CreateSale(p.ID, 2); err != nil { // 300
		t.Fatalf("create sale: %v", err)
	}
	if _, err := a.store.CreateBilliardEntry(1000, "Table 1", nil, ""); err != nil {
		t.Fatalf("create billiard entry: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/summary", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if body["billiard_revenue_today"].(float64) != 1000 {
		t.Fatalf("billiard_revenue_today = %v, want 1000", body["billiard_revenue_today"])
	}
	if body["billiard_count_today"].(float64) != 1 {
		t.Fatalf("billiard_count_today = %v, want 1", body["billiard_count_today"])
	}
	if body["total_revenue_today"].(float64) != 1300 {
		t.Fatalf("total_revenue_today = %v, want 1300 (300 shop + 1000 billiard)", body["total_revenue_today"])
	}
}

func TestDailySalesIncludesBilliardAndTotalRevenue(t *testing.T) {
	a := newTestAPI(t)

	p, err := a.store.CreateProduct("Cola", "", 150, 100, 5)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	mustSell(t, a, p.ID, 2) // 300
	if _, err := a.store.CreateBilliardEntry(700, "Table 2", nil, ""); err != nil {
		t.Fatalf("create billiard entry: %v", err)
	}

	res, body := doDailyRequest(t, a, "")
	if res.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want %d", res.StatusCode, http.StatusOK)
	}
	if body.Billiard.Revenue != 700 || body.Billiard.Count != 1 {
		t.Fatalf("billiard = %+v, want revenue 700 count 1", body.Billiard)
	}
	if len(body.Billiard.Entries) != 1 {
		t.Fatalf("len(billiard entries) = %d, want 1", len(body.Billiard.Entries))
	}
	if body.Totals.TotalRevenue != 1000 {
		t.Fatalf("totals.total_revenue = %d, want 1000 (300 shop + 700 billiard)", body.Totals.TotalRevenue)
	}
}

func TestSalesChartZeroFillsBilliardRevenue(t *testing.T) {
	a := newTestAPI(t)

	if _, err := a.store.CreateBilliardEntry(900, "Table 1", nil, ""); err != nil {
		t.Fatalf("create billiard entry: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/sales/chart?days=7", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}

	var body []chartEntryDTO
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(body) != 7 {
		t.Fatalf("len(body) = %d, want 7", len(body))
	}
	last := body[6]
	if last.BilliardRevenue != 900 {
		t.Fatalf("today's billiard_revenue = %d, want 900", last.BilliardRevenue)
	}
	for i := 0; i < 6; i++ {
		if body[i].BilliardRevenue != 0 {
			t.Fatalf("body[%d].billiard_revenue = %d, want 0", i, body[i].BilliardRevenue)
		}
	}
}
