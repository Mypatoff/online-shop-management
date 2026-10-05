package api

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"shop/internal/db"
	"shop/internal/store"
)

func newTestAPI(t *testing.T) *API {
	t.Helper()
	conn, err := db.Open(":memory:", "")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return New(store.New(conn), "Test Shop", "USD", 2)
}

func TestWrongHostRejected(t *testing.T) {
	a := newTestAPI(t)
	handler := a.Routes(8080)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Host = "evil.example.com:8080"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusForbidden)
	}
}

func TestAllowedHostSucceeds(t *testing.T) {
	a := newTestAPI(t)
	handler := a.Routes(8080)

	req := httptest.NewRequest(http.MethodGet, "/api/config", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}
