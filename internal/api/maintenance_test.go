package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"shop/internal/db"
	"shop/internal/store"
)

// maintEnv is an API on a real database file with a backup folder.
type maintEnv struct {
	api       *API
	handler   http.Handler
	conn      *sql.DB
	dbPath    string
	backupDir string
	restarted chan bool // receives finalBackup when Restart is called
	quit      atomic.Bool
}

func newMaintEnv(t *testing.T) *maintEnv {
	t.Helper()
	dir := t.TempDir()
	e := &maintEnv{
		dbPath:    filepath.Join(dir, "shop.db"),
		backupDir: filepath.Join(dir, "backups"),
		restarted: make(chan bool, 1),
	}
	if err := os.MkdirAll(e.backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	conn, err := db.Open(e.dbPath, e.backupDir)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close() })
	e.conn = conn
	e.api = New(store.New(conn), "Test", "so'm", 0)
	e.api.SetMaintenance(Maintenance{
		DBPath: e.dbPath, BackupDir: e.backupDir,
		Restart: func(final bool) { e.restarted <- final },
		Quit:    func() { e.quit.Store(true) },
	})
	e.handler = e.api.Routes(8080)
	return e
}

func (e *maintEnv) do(method, path, contentType string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Host = "127.0.0.1:8080"
	if contentType != "" {
		req.Header.Set("Content-Type", contentType)
	}
	rec := httptest.NewRecorder()
	e.handler.ServeHTTP(rec, req)
	return rec
}

func (e *maintEnv) addProduct(t *testing.T, name string) {
	t.Helper()
	if _, err := e.conn.Exec(`INSERT INTO products (name, price, stock, created_at, updated_at) VALUES (?, 1, 1, 1, 1)`, name); err != nil {
		t.Fatal(err)
	}
}

func (e *maintEnv) waitRestart(t *testing.T) bool {
	t.Helper()
	select {
	case final := <-e.restarted:
		return final
	case <-time.After(3 * time.Second):
		t.Fatal("the app was not asked to restart")
		return false
	}
}

func (e *maintEnv) noRestart(t *testing.T) {
	t.Helper()
	select {
	case <-e.restarted:
		t.Fatal("the app was asked to restart after a failed restore")
	case <-time.After(800 * time.Millisecond):
	}
}

func jsonBody(v any) []byte { b, _ := json.Marshal(v); return b }

func TestListBackups(t *testing.T) {
	e := newMaintEnv(t)
	if err := e.api.store.BackupTo(filepath.Join(e.backupDir, "shop-2026-01-01.db")); err != nil {
		t.Fatal(err)
	}
	rec := e.do("GET", "/api/backups", "", nil)
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	var got struct {
		Backups []backupFileDTO `json:"backups"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Backups) != 1 || got.Backups[0].Name != "shop-2026-01-01.db" || got.Backups[0].Size == 0 {
		t.Fatalf("backups = %+v", got.Backups)
	}
}

func TestRestoreByNameStagesBacksUpAndRestarts(t *testing.T) {
	e := newMaintEnv(t)
	e.addProduct(t, "Before")
	name := "shop-2026-01-01.db"
	if err := e.api.store.BackupTo(filepath.Join(e.backupDir, name)); err != nil {
		t.Fatal(err)
	}
	e.addProduct(t, "After")

	rec := e.do("POST", "/api/restore", "application/json", jsonBody(map[string]string{"name": name}))
	if rec.Code != 200 {
		t.Fatalf("status %d: %s", rec.Code, rec.Body)
	}
	if final := e.waitRestart(t); final {
		t.Error("restore must skip the final backup (it would overwrite today's daily file)")
	}

	if _, err := os.Stat(e.dbPath + ".restore"); err != nil {
		t.Errorf("no staged restore: %v", err)
	}
	// The data as it was just before the restore must be kept.
	matches, _ := filepath.Glob(filepath.Join(e.backupDir, "shop-prerestore-*.db"))
	if len(matches) != 1 {
		t.Fatalf("pre-restore backups = %v, want exactly one", matches)
	}
	safety, err := sql.Open("sqlite", matches[0])
	if err != nil {
		t.Fatal(err)
	}
	defer safety.Close()
	var n int
	if err := safety.QueryRow(`SELECT count(*) FROM products`).Scan(&n); err != nil || n != 2 {
		t.Errorf("safety backup holds %d products (err %v), want the 2 present before the restore", n, err)
	}
	// Until the app restarts, the live database is untouched.
	if err := e.conn.QueryRow(`SELECT count(*) FROM products`).Scan(&n); err != nil || n != 2 {
		t.Errorf("live database changed before the restart: %d", n)
	}
}

func TestRestoreRejections(t *testing.T) {
	e := newMaintEnv(t)
	bogus := filepath.Join(e.backupDir, "bogus.db")
	if err := os.WriteFile(bogus, []byte("not a database"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Placed outside the backup folder: must not be reachable by name.
	outside := filepath.Join(filepath.Dir(e.backupDir), "outside.db")
	if err := e.api.store.BackupTo(outside); err != nil {
		t.Fatal(err)
	}

	for name, req := range map[string]string{
		"not a database":  "bogus.db",
		"traversal":       "../outside.db",
		"absolute":        outside,
		"missing":         "nope.db",
		"empty":           "",
		"wrong extension": "shop.txt",
	} {
		rec := e.do("POST", "/api/restore", "application/json", jsonBody(map[string]string{"name": req}))
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: status %d, want 400 (%s)", name, rec.Code, rec.Body)
		}
	}
	e.noRestart(t)
	if _, err := os.Stat(e.dbPath + ".restore"); err == nil {
		t.Error("a rejected restore left a staged file")
	}
	if m, _ := filepath.Glob(filepath.Join(e.backupDir, "shop-prerestore-*.db")); len(m) != 0 {
		t.Errorf("rejected restores must not create safety backups: %v", m)
	}
}

func TestRestoreUpload(t *testing.T) {
	e := newMaintEnv(t)
	e.addProduct(t, "Uploaded")
	src := filepath.Join(t.TempDir(), "downloaded.db")
	if err := e.api.store.BackupTo(src); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(src)
	if err != nil {
		t.Fatal(err)
	}

	// Garbage is refused...
	rec := e.do("POST", restoreUploadPath, "application/octet-stream", []byte("hello"))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("garbage upload: status %d, want 400 (%s)", rec.Code, rec.Body)
	}
	e.noRestart(t)

	// ...a wrong content type is refused...
	if rec := e.do("POST", restoreUploadPath, "application/json", data); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("json upload: status %d, want 415", rec.Code)
	}
	// ...and the octet-stream exemption does not leak to other endpoints.
	if rec := e.do("POST", "/api/restore", "application/octet-stream", data); rec.Code != http.StatusUnsupportedMediaType {
		t.Errorf("octet-stream to /api/restore: status %d, want 415", rec.Code)
	}

	// A real backup is accepted, staged and the restart requested.
	rec = e.do("POST", restoreUploadPath, "application/octet-stream", data)
	if rec.Code != 200 {
		t.Fatalf("upload: status %d: %s", rec.Code, rec.Body)
	}
	e.waitRestart(t)
	if _, err := os.Stat(e.dbPath + ".restore"); err != nil {
		t.Errorf("upload was not staged: %v", err)
	}
	if m, _ := filepath.Glob(filepath.Join(e.backupDir, "upload-*.tmp")); len(m) != 0 {
		t.Errorf("upload temp file left behind: %v", m)
	}
}

func TestUploadBeyondJSONLimitIsAllowedUpToUploadLimit(t *testing.T) {
	e := newMaintEnv(t)
	// Body bigger than the 64 KiB JSON cap: must reach the validator (400 for
	// not being a database), not be cut off by the body limit (413).
	rec := e.do("POST", restoreUploadPath, "application/octet-stream", bytes.Repeat([]byte("x"), 3*maxBodyBytes))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status %d, want 400 from validation: %s", rec.Code, rec.Body)
	}
	if !strings.Contains(rec.Body.String(), "SQLite") {
		t.Errorf("unexpected message: %s", rec.Body)
	}
}

func TestQuitAndConfig(t *testing.T) {
	e := newMaintEnv(t)
	var cfg map[string]any
	if err := json.Unmarshal(e.do("GET", "/api/config", "", nil).Body.Bytes(), &cfg); err != nil {
		t.Fatal(err)
	}
	if cfg["can_quit"] != true || cfg["lang"] != "en" {
		t.Errorf("config = %v", cfg)
	}
	if rec := e.do("POST", "/api/quit", "application/json", []byte("{}")); rec.Code != 200 {
		t.Fatalf("quit: %d", rec.Code)
	}
	deadline := time.Now().Add(3 * time.Second)
	for !e.quit.Load() && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if !e.quit.Load() {
		t.Error("Quit was not called")
	}

	// No Quit hook (the window build): refused, and config says so.
	e.api.SetMaintenance(Maintenance{})
	if rec := e.do("POST", "/api/quit", "application/json", []byte("{}")); rec.Code != http.StatusNotImplemented {
		t.Errorf("quit without hook: %d, want 501", rec.Code)
	}
	if err := json.Unmarshal(e.do("GET", "/api/config", "", nil).Body.Bytes(), &cfg); err != nil || cfg["can_quit"] != false {
		t.Errorf("config without quit = %v (%v)", cfg, err)
	}
}

func TestCrossOriginCannotRestoreOrQuit(t *testing.T) {
	e := newMaintEnv(t)
	for _, path := range []string{"/api/quit", "/api/restore", "/api/update/apply"} {
		req := httptest.NewRequest("POST", path, strings.NewReader("{}"))
		req.Host = "127.0.0.1:8080"
		req.Header.Set("Origin", "https://evil.example")
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		e.handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusForbidden {
			t.Errorf("%s from another origin: status %d, want 403", path, rec.Code)
		}
	}
}
