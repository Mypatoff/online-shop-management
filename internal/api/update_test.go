package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"shop/internal/update"
)

func fakeGitHub(t *testing.T, tag string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/repos/o/r/releases/latest" {
			http.NotFound(w, r)
			return
		}
		fmt.Fprintf(w, `{"tag_name":%q,"html_url":"https://example/r","assets":[]}`, tag)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func get(t *testing.T, a *API, path string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(rec, req)
	return rec
}

func TestUpdateRoutesOffWithoutUpdater(t *testing.T) {
	a := newTestAPI(t)
	for _, path := range []string{"/api/version", "/api/update/check", "/api/update/status"} {
		if rec := get(t, a, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}

func TestVersionAndUpdateCheck(t *testing.T) {
	gh := fakeGitHub(t, "v2.0.0")
	u := update.New("1.0.0")
	u.Repo, u.APIBase, u.Client = "o/r", gh.URL, gh.Client()
	a := newTestAPI(t).WithUpdater(u)

	rec := get(t, a, "/api/version")
	var ver map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &ver); err != nil || ver["version"] != "1.0.0" {
		t.Fatalf("/api/version = %d %s", rec.Code, rec.Body)
	}

	rec = get(t, a, "/api/update/check")
	var info update.Info
	if err := json.Unmarshal(rec.Body.Bytes(), &info); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("/api/update/check = %d %s", rec.Code, rec.Body)
	}
	if !info.Available || info.Latest != "2.0.0" {
		t.Fatalf("info = %+v", info)
	}

	rec = get(t, a, "/api/update/status")
	var st update.Status
	if err := json.Unmarshal(rec.Body.Bytes(), &st); err != nil || st.State != "idle" {
		t.Fatalf("/api/update/status = %d %s", rec.Code, rec.Body)
	}
}

func TestUpdateCheckOfflineIsReadableError(t *testing.T) {
	gh := fakeGitHub(t, "v2.0.0")
	gh.Close() // nothing is listening any more
	u := update.New("1.0.0")
	u.Repo, u.APIBase, u.Client = "o/r", gh.URL, gh.Client()
	a := newTestAPI(t).WithUpdater(u)

	rec := get(t, a, "/api/update/check")
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502; body %s", rec.Code, rec.Body)
	}
}

func TestUpdateApplyRequiresJSONAndLocalOrigin(t *testing.T) {
	u := update.New("1.0.0")
	a := newTestAPI(t).WithUpdater(u)

	req := httptest.NewRequest(http.MethodPost, "/api/update/apply", nil)
	req.Host = "127.0.0.1:8080"
	rec := httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(rec, req)
	if rec.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("no content type: status = %d, want 415", rec.Code)
	}

	req = httptest.NewRequest(http.MethodPost, "/api/update/apply", nil)
	req.Host = "127.0.0.1:8080"
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", "https://evil.example")
	rec = httptest.NewRecorder()
	a.Routes(8080).ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("foreign origin: status = %d, want 403", rec.Code)
	}
}
