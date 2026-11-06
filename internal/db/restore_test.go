package db

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// newShop creates a database at dir/shop.db holding one product named name.
func newShop(t *testing.T, dir, name string) (*sql.DB, string) {
	t.Helper()
	path := filepath.Join(dir, "shop.db")
	conn, err := Open(path, filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := conn.Exec(
		`INSERT INTO products (name, price, stock, created_at, updated_at) VALUES (?, 100, 1, 1, 1)`, name); err != nil {
		t.Fatalf("seed: %v", err)
	}
	return conn, path
}

func productNames(t *testing.T, path string) []string {
	t.Helper()
	conn, err := Open(path, filepath.Join(filepath.Dir(path), "backups"))
	if err != nil {
		t.Fatalf("reopen %s: %v", path, err)
	}
	defer conn.Close()
	rows, err := conn.Query(`SELECT name FROM products ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var names []string
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		names = append(names, n)
	}
	return names
}

func TestRestoreRoundTrip(t *testing.T) {
	dir := t.TempDir()
	conn, dbPath := newShop(t, dir, "Old")

	backup := filepath.Join(dir, "shop-2026-01-01.db")
	if err := Backup(conn, backup); err != nil {
		t.Fatal(err)
	}
	// Data written after the backup is what a restore should discard.
	if _, err := conn.Exec(
		`INSERT INTO products (name, price, stock, created_at, updated_at) VALUES ('New', 1, 1, 1, 1)`); err != nil {
		t.Fatal(err)
	}

	if err := StageRestore(dbPath, backup); err != nil {
		t.Fatalf("StageRestore: %v", err)
	}
	// Staging alone must not touch the live database.
	var n int
	if err := conn.QueryRow(`SELECT count(*) FROM products`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("live database after staging: count=%d err=%v, want 2", n, err)
	}
	conn.Close() // the app exits before the swap

	applied, err := ApplyPendingRestore(dbPath)
	if err != nil || !applied {
		t.Fatalf("ApplyPendingRestore = %v, %v; want true, nil", applied, err)
	}
	if got := productNames(t, dbPath); len(got) != 1 || got[0] != "Old" {
		t.Fatalf("products after restore = %v, want [Old]", got)
	}
	if _, err := os.Stat(backup); err != nil {
		t.Fatalf("the backup file must survive a restore: %v", err)
	}
	if _, err := os.Stat(dbPath + ".restore"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("staged file should be gone after applying, stat err = %v", err)
	}
	if again, err := ApplyPendingRestore(dbPath); again || err != nil {
		t.Fatalf("second ApplyPendingRestore = %v, %v; want false, nil", again, err)
	}
}

func TestApplyPendingRestoreDropsOldWAL(t *testing.T) {
	dir := t.TempDir()
	conn, dbPath := newShop(t, dir, "Old")
	backup := filepath.Join(dir, "b.db")
	if err := Backup(conn, backup); err != nil {
		t.Fatal(err)
	}
	if err := StageRestore(dbPath, backup); err != nil {
		t.Fatal(err)
	}
	// The app has exited by the time a restore is applied. Close first: Windows
	// will not let the sidecar files be written while the database maps them.
	conn.Close()
	// A leftover WAL from the old database must not survive the swap, or it
	// would be replayed onto the restored file.
	for _, ext := range []string{"-wal", "-shm"} {
		if err := os.WriteFile(dbPath+ext, []byte("stale"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := ApplyPendingRestore(dbPath); err != nil {
		t.Fatal(err)
	}
	for _, ext := range []string{"-wal", "-shm"} {
		if _, err := os.Stat(dbPath + ext); !errors.Is(err, os.ErrNotExist) {
			t.Errorf("%s survived the restore (stat err = %v)", ext, err)
		}
	}
}

func TestStageRestoreRejectsBadFiles(t *testing.T) {
	dir := t.TempDir()
	conn, dbPath := newShop(t, dir, "Old")
	defer conn.Close()

	notDB := filepath.Join(dir, "notes.db")
	if err := os.WriteFile(notDB, []byte("this is not a database, just text"), 0o644); err != nil {
		t.Fatal(err)
	}

	empty := filepath.Join(dir, "empty.db")
	if err := os.WriteFile(empty, nil, 0o644); err != nil {
		t.Fatal(err)
	}

	noTables := filepath.Join(dir, "other.db")
	other, err := sql.Open("sqlite", noTables)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := other.Exec(`CREATE TABLE unrelated (x INTEGER)`); err != nil {
		t.Fatal(err)
	}
	other.Close()

	// A real ShopKeeper database from a future release.
	future := filepath.Join(dir, "future.db")
	if err := Backup(conn, future); err != nil {
		t.Fatal(err)
	}
	f, err := sql.Open("sqlite", future)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	f.Close()

	// A real database with its pages damaged.
	damaged := filepath.Join(dir, "damaged.db")
	if err := Backup(conn, damaged); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(damaged)
	if err != nil {
		t.Fatal(err)
	}
	for i := 4096; i < len(raw); i++ {
		raw[i] = 0xFF
	}
	if err := os.WriteFile(damaged, raw, 0o644); err != nil {
		t.Fatal(err)
	}

	for name, path := range map[string]string{
		"text file": notDB, "empty file": empty, "no shop tables": noTables,
		"newer schema": future, "damaged": damaged,
	} {
		err := StageRestore(dbPath, path)
		if !errors.Is(err, ErrInvalidBackup) {
			t.Errorf("%s: StageRestore error = %v, want ErrInvalidBackup", name, err)
		}
		if _, statErr := os.Stat(dbPath + ".restore"); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("%s: a rejected file left a staged restore behind", name)
		}
		if _, statErr := os.Stat(dbPath + ".restore.tmp"); !errors.Is(statErr, os.ErrNotExist) {
			t.Errorf("%s: a rejected file left a temp copy behind", name)
		}
	}
}

func TestStageRestoreAcceptsOlderSchemaAndMigratesOnOpen(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shop.db")
	old := filepath.Join(dir, "old.db")
	makeV3SchemaDB(t, old)

	if err := StageRestore(dbPath, old); err != nil {
		t.Fatalf("an older-schema backup must be restorable: %v", err)
	}
	if _, err := ApplyPendingRestore(dbPath); err != nil {
		t.Fatal(err)
	}
	conn, err := Open(dbPath, filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatalf("Open restored v3 database: %v", err)
	}
	defer conn.Close()
	var v int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&v); err != nil || v != CurrentVersion {
		t.Fatalf("user_version after open = %d (err %v), want %d", v, err, CurrentVersion)
	}
}

func TestBackupPathRejectsEscapes(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "shop-2026-01-01.db"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := BackupPath(dir, "shop-2026-01-01.db"); err != nil {
		t.Errorf("valid name rejected: %v", err)
	}
	for _, bad := range []string{"", "../shop.db", "sub/shop.db", `..\shop.db`, "shop.txt", "missing.db", "."} {
		if _, err := BackupPath(dir, bad); !errors.Is(err, ErrInvalidBackup) {
			t.Errorf("BackupPath(%q) error = %v, want ErrInvalidBackup", bad, err)
		}
	}
}

func TestListBackupsNewestFirstSkipsJunk(t *testing.T) {
	dir := t.TempDir()
	base := time.Now()
	for i, name := range []string{"a.db", "b.db", "c.db"} {
		p := filepath.Join(dir, name)
		if err := os.WriteFile(p, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		mt := base.Add(time.Duration(i) * time.Hour)
		if err := os.Chtimes(p, mt, mt); err != nil {
			t.Fatal(err)
		}
	}
	for _, junk := range []string{"shop-backup-1.db.tmp", "readme.txt"} {
		if err := os.WriteFile(filepath.Join(dir, junk), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "sub.db"), 0o755); err != nil {
		t.Fatal(err)
	}

	files, err := ListBackups(dir)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range files {
		got = append(got, f.Name)
	}
	if len(got) != 3 || got[0] != "c.db" || got[2] != "a.db" {
		t.Fatalf("ListBackups = %v, want [c.db b.db a.db]", got)
	}

	if files, err := ListBackups(filepath.Join(dir, "nope")); err != nil || len(files) != 0 {
		t.Fatalf("missing dir: %v, %v; want empty, nil", files, err)
	}
}

func TestBackupLeavesNoTempFilesAndSurvivesConcurrentCalls(t *testing.T) {
	dir := t.TempDir()
	conn, _ := newShop(t, dir, "x")
	defer conn.Close()
	out := filepath.Join(dir, "out")
	if err := os.Mkdir(out, 0o755); err != nil {
		t.Fatal(err)
	}

	done := make(chan error, 8)
	for i := 0; i < 8; i++ {
		go func() { done <- Backup(conn, filepath.Join(out, "same.db")) }()
	}
	for i := 0; i < 8; i++ {
		if err := <-done; err != nil {
			t.Fatalf("concurrent Backup: %v", err)
		}
	}
	entries, err := os.ReadDir(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "same.db" {
		t.Fatalf("backup dir = %v, want only same.db (no temp leftovers)", entries)
	}
}
