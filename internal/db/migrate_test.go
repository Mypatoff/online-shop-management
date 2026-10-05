package db

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	_ "modernc.org/sqlite"
)

// oldSchemaProductsAndSales is what the database looked like before
// stock_movements and PRAGMA user_version migrations existed.
const oldSchemaProductsAndSales = `
CREATE TABLE products (
	id                  INTEGER PRIMARY KEY AUTOINCREMENT,
	name                TEXT NOT NULL,
	sku                 TEXT UNIQUE,
	price               INTEGER NOT NULL CHECK (price >= 0),
	stock               INTEGER NOT NULL DEFAULT 0 CHECK (stock >= 0),
	low_stock_threshold INTEGER NOT NULL DEFAULT 5,
	archived            INTEGER NOT NULL DEFAULT 0,
	created_at          INTEGER NOT NULL,
	updated_at          INTEGER NOT NULL
);

CREATE TABLE sales (
	id           INTEGER PRIMARY KEY AUTOINCREMENT,
	product_id   INTEGER NOT NULL REFERENCES products(id),
	product_name TEXT NOT NULL,
	quantity     INTEGER NOT NULL CHECK (quantity > 0),
	unit_price   INTEGER NOT NULL,
	total        INTEGER NOT NULL,
	created_at   INTEGER NOT NULL,
	voided_at    INTEGER
);
`

// makeOldSchemaDB creates a database file at path with the pre-
// versioning schema (no stock_movements, PRAGMA user_version left at
// its default 0) and one seeded product, simulating a real database
// from before this migration existed.
func makeOldSchemaDB(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open old-schema db: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Exec(oldSchemaProductsAndSales); err != nil {
		t.Fatalf("create old schema: %v", err)
	}
	if _, err := conn.Exec(
		`INSERT INTO products (name, sku, price, stock, low_stock_threshold, archived, created_at, updated_at)
		 VALUES ('Cola', NULL, 150, 10, 5, 0, 1000, 1000)`,
	); err != nil {
		t.Fatalf("seed product: %v", err)
	}
}

func TestMigrateExistingDBBackfillsAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shop.db")
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}

	makeOldSchemaDB(t, dbPath)

	conn, err := Open(dbPath, backupDir)
	if err != nil {
		t.Fatalf("Open (migrate): %v", err)
	}
	defer conn.Close()

	var version int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != currentVersion {
		t.Fatalf("user_version = %d, want %d", version, currentVersion)
	}

	rows, err := conn.Query(`SELECT type, delta, stock_after, note FROM stock_movements`)
	if err != nil {
		t.Fatalf("query stock_movements: %v", err)
	}
	defer rows.Close()
	var count int
	for rows.Next() {
		var mtype, note string
		var delta, stockAfter int64
		if err := rows.Scan(&mtype, &delta, &stockAfter, &note); err != nil {
			t.Fatalf("scan movement: %v", err)
		}
		if mtype != "initial" || delta != 10 || stockAfter != 10 || note != "Before history tracking" {
			t.Fatalf("movement = (%s, %d, %d, %q), want (initial, 10, 10, \"Before history tracking\")", mtype, delta, stockAfter, note)
		}
		count++
	}
	if count != 1 {
		t.Fatalf("stock_movements row count = %d, want 1", count)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	var foundPremigration bool
	for _, e := range entries {
		if matched, _ := filepath.Match("shop-premigration-*.db", e.Name()); matched {
			foundPremigration = true
		}
	}
	if !foundPremigration {
		t.Fatalf("no shop-premigration-*.db found in %s, entries: %v", backupDir, entries)
	}
}

func TestMigrateTwiceAddsNothing(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shop.db")
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}

	makeOldSchemaDB(t, dbPath)

	conn1, err := Open(dbPath, backupDir)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	var countAfterFirst int
	if err := conn1.QueryRow(`SELECT count(*) FROM stock_movements`).Scan(&countAfterFirst); err != nil {
		t.Fatalf("count after first open: %v", err)
	}
	conn1.Close()

	conn2, err := Open(dbPath, backupDir)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer conn2.Close()
	var countAfterSecond int
	if err := conn2.QueryRow(`SELECT count(*) FROM stock_movements`).Scan(&countAfterSecond); err != nil {
		t.Fatalf("count after second open: %v", err)
	}

	if countAfterSecond != countAfterFirst {
		t.Fatalf("stock_movements count changed on second Open: %d -> %d", countAfterFirst, countAfterSecond)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	var premigrationCount int
	for _, e := range entries {
		if matched, _ := filepath.Match("shop-premigration-*.db", e.Name()); matched {
			premigrationCount++
		}
	}
	if premigrationCount != 1 {
		t.Fatalf("premigration backup count = %d, want 1 (second Open shouldn't take another)", premigrationCount)
	}
}

// makeV2SchemaDB creates a database file already at user_version 2
// (products, sales and stock_movements present, no billiard_entries
// yet), simulating a real database from just before this migration
// existed.
func makeV2SchemaDB(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open v2-schema db: %v", err)
	}
	defer conn.Close()

	if _, err := conn.Exec(oldSchemaProductsAndSales); err != nil {
		t.Fatalf("create base schema: %v", err)
	}
	if _, err := conn.Exec(stockMovementsSchema); err != nil {
		t.Fatalf("create stock_movements: %v", err)
	}
	if _, err := conn.Exec(
		`INSERT INTO products (name, sku, price, stock, low_stock_threshold, archived, created_at, updated_at)
		 VALUES ('Cola', NULL, 150, 10, 5, 0, 1000, 1000)`,
	); err != nil {
		t.Fatalf("seed product: %v", err)
	}
	if _, err := conn.Exec(`PRAGMA user_version = 2`); err != nil {
		t.Fatalf("set user_version: %v", err)
	}
}

func TestMigrateV2ToV3CreatesBilliardTableAndKeepsData(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shop.db")
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}

	makeV2SchemaDB(t, dbPath)

	conn, err := Open(dbPath, backupDir)
	if err != nil {
		t.Fatalf("Open (migrate): %v", err)
	}
	defer conn.Close()

	var version int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != currentVersion {
		t.Fatalf("user_version = %d, want %d", version, currentVersion)
	}

	var billiardCount int
	if err := conn.QueryRow(`SELECT count(*) FROM billiard_entries`).Scan(&billiardCount); err != nil {
		t.Fatalf("query billiard_entries (table should exist): %v", err)
	}
	if billiardCount != 0 {
		t.Fatalf("billiard_entries count = %d, want 0", billiardCount)
	}

	var productCount int
	if err := conn.QueryRow(`SELECT count(*) FROM products`).Scan(&productCount); err != nil {
		t.Fatalf("query products: %v", err)
	}
	if productCount != 1 {
		t.Fatalf("existing product data lost: count = %d, want 1", productCount)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	var foundPremigration bool
	for _, e := range entries {
		if matched, _ := filepath.Match("shop-premigration-*.db", e.Name()); matched {
			foundPremigration = true
		}
	}
	if !foundPremigration {
		t.Fatalf("no shop-premigration-*.db found in %s, entries: %v", backupDir, entries)
	}
}

func TestMigrateV2ToV3TwiceAddsNothing(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shop.db")
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}

	makeV2SchemaDB(t, dbPath)

	conn1, err := Open(dbPath, backupDir)
	if err != nil {
		t.Fatalf("first Open: %v", err)
	}
	conn1.Close()

	conn2, err := Open(dbPath, backupDir)
	if err != nil {
		t.Fatalf("second Open: %v", err)
	}
	defer conn2.Close()

	var version int
	if err := conn2.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != currentVersion {
		t.Fatalf("user_version = %d, want %d", version, currentVersion)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	var premigrationCount int
	for _, e := range entries {
		if matched, _ := filepath.Match("shop-premigration-*.db", e.Name()); matched {
			premigrationCount++
		}
	}
	if premigrationCount != 1 {
		t.Fatalf("premigration backup count = %d, want 1 (second Open shouldn't take another)", premigrationCount)
	}
}

func TestNewDatabaseSkipsMigrationAndBackup(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shop.db")
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatalf("mkdir backups: %v", err)
	}

	conn, err := Open(dbPath, backupDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	var version int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatalf("read user_version: %v", err)
	}
	if version != currentVersion {
		t.Fatalf("user_version = %d, want %d", version, currentVersion)
	}

	var billiardCount int
	if err := conn.QueryRow(`SELECT count(*) FROM billiard_entries`).Scan(&billiardCount); err != nil {
		t.Fatalf("query billiard_entries (table should exist on a brand-new db): %v", err)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatalf("read backup dir: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("backup dir should be empty for a brand-new database, got %v", entries)
	}
}
