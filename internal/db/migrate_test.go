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

// makeV3SchemaDB creates a database file at user_version 3: everything but
// the v4 indexes, with a sale and a stock movement to carry across.
func makeV3SchemaDB(t *testing.T, path string) {
	t.Helper()
	conn, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatalf("open v3-schema db: %v", err)
	}
	defer conn.Close()

	for _, stmt := range []string{
		oldSchemaProductsAndSales,
		// The pre-v4 versions of these two, without the indexes added later.
		`CREATE TABLE stock_movements (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			product_id INTEGER NOT NULL REFERENCES products(id),
			type TEXT NOT NULL CHECK (type IN ('initial', 'sale', 'void', 'adjust')),
			delta INTEGER NOT NULL, stock_after INTEGER NOT NULL,
			reason TEXT, note TEXT, sale_id INTEGER REFERENCES sales(id),
			created_at INTEGER NOT NULL)`,
		`CREATE TABLE billiard_entries (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			amount INTEGER NOT NULL CHECK (amount > 0),
			table_name TEXT, minutes INTEGER, note TEXT,
			created_at INTEGER NOT NULL, voided_at INTEGER)`,
		`CREATE INDEX idx_stock_movements_product_created ON stock_movements(product_id, created_at)`,
		`CREATE INDEX idx_billiard_entries_created ON billiard_entries(created_at)`,
		`INSERT INTO products (name, sku, price, stock, low_stock_threshold, archived, created_at, updated_at)
		 VALUES ('Cola', NULL, 150, 9, 5, 0, 1000, 1000)`,
		`INSERT INTO sales (product_id, product_name, quantity, unit_price, total, created_at)
		 VALUES (1, 'Cola', 1, 150, 150, 2000)`,
		`INSERT INTO stock_movements (product_id, type, delta, stock_after, sale_id, created_at)
		 VALUES (1, 'sale', -1, 9, 1, 2000)`,
		`PRAGMA user_version = 3`,
	} {
		if _, err := conn.Exec(stmt); err != nil {
			t.Fatalf("seed v3 db (%.40s...): %v", stmt, err)
		}
	}
}

func indexNames(t *testing.T, conn *sql.DB) map[string]bool {
	t.Helper()
	rows, err := conn.Query(`SELECT name FROM sqlite_master WHERE type = 'index' AND name LIKE 'idx_%'`)
	if err != nil {
		t.Fatalf("list indexes: %v", err)
	}
	defer rows.Close()
	names := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatalf("scan index name: %v", err)
		}
		names[n] = true
	}
	return names
}

var wantV4Indexes = []string{
	"idx_products_archived_name",
	"idx_sales_created",
	"idx_sales_product_created",
	"idx_stock_movements_sale",
	"idx_stock_movements_product_created",
	"idx_billiard_entries_created",
}

func TestMigrateV3ToV4AddsIndexesBacksUpAndKeepsData(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shop.db")
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	makeV3SchemaDB(t, dbPath)

	conn, err := Open(dbPath, backupDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	var version int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		t.Fatal(err)
	}
	if version != currentVersion {
		t.Fatalf("user_version = %d, want %d", version, currentVersion)
	}

	got := indexNames(t, conn)
	for _, name := range wantV4Indexes {
		if !got[name] {
			t.Errorf("index %s missing after migration; have %v", name, got)
		}
	}

	var products, sales, movements int
	if err := conn.QueryRow(`SELECT (SELECT count(*) FROM products), (SELECT count(*) FROM sales), (SELECT count(*) FROM stock_movements)`).
		Scan(&products, &sales, &movements); err != nil {
		t.Fatal(err)
	}
	if products != 1 || sales != 1 || movements != 1 {
		t.Fatalf("rows after migration: products=%d sales=%d movements=%d, want 1 each", products, sales, movements)
	}

	entries, err := os.ReadDir(backupDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("backups after migrating an existing database = %v, want exactly one pre-migration backup", entries)
	}
}

func TestMigrateV3ToV4IsRepeatable(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shop.db")
	makeV3SchemaDB(t, dbPath)

	conn, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()

	// Run the step twice, as if it had been interrupted after the indexes were
	// created but before user_version was recorded.
	for i := 0; i < 2; i++ {
		if err := migrateV3ToV4(conn); err != nil {
			t.Fatalf("migrateV3ToV4 run %d: %v", i+1, err)
		}
	}
}

func TestNewDatabaseHasV4Indexes(t *testing.T) {
	dir := t.TempDir()
	conn, err := Open(filepath.Join(dir, "shop.db"), filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	got := indexNames(t, conn)
	for _, name := range wantV4Indexes {
		if !got[name] {
			t.Errorf("index %s missing on a brand-new database; have %v", name, got)
		}
	}
}

func TestOpenRefusesNewerSchema(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shop.db")
	conn, err := Open(dbPath, filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Exec(`PRAGMA user_version = 99`); err != nil {
		t.Fatal(err)
	}
	conn.Close()

	if conn, err := Open(dbPath, filepath.Join(dir, "backups")); err == nil {
		conn.Close()
		t.Fatal("Open succeeded on a database from a newer version, want an error")
	}
}

func columnNames(t *testing.T, conn *sql.DB, table string) map[string]bool {
	t.Helper()
	rows, err := conn.Query(`SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	cols := map[string]bool{}
	for rows.Next() {
		var n string
		if err := rows.Scan(&n); err != nil {
			t.Fatal(err)
		}
		cols[n] = true
	}
	return cols
}

func TestMigrateV4ToV5AddsCupsKeepsSalesAndBacksUp(t *testing.T) {
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "shop.db")
	backupDir := filepath.Join(dir, "backups")
	if err := os.MkdirAll(backupDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A real v4 database: the v3 fixture taken through the v3 -> v4 step.
	makeV3SchemaDB(t, dbPath)
	raw, err := sql.Open("sqlite", dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := migrateV3ToV4(raw); err != nil {
		t.Fatal(err)
	}
	raw.Close()

	conn, err := Open(dbPath, backupDir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer conn.Close()

	var version int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 5 {
		t.Fatalf("user_version = %d (err %v), want 5", version, err)
	}
	cols := columnNames(t, conn, "sales")
	if !cols["cups"] || !cols["cup_price"] {
		t.Fatalf("sales columns = %v, want cups and cup_price", cols)
	}
	var total, cups, cupPrice int64
	if err := conn.QueryRow(`SELECT total, cups, cup_price FROM sales WHERE id = 1`).Scan(&total, &cups, &cupPrice); err != nil {
		t.Fatal(err)
	}
	if total != 150 || cups != 0 || cupPrice != 0 {
		t.Fatalf("existing sale = total %d cups %d cup_price %d, want 150, 0, 0", total, cups, cupPrice)
	}
	if _, err := conn.Exec(`INSERT INTO settings (key, value) VALUES ('cup_price', '500')`); err != nil {
		t.Fatalf("settings table missing: %v", err)
	}
	if entries, _ := os.ReadDir(backupDir); len(entries) != 1 {
		t.Fatalf("backups = %v, want exactly one pre-migration backup", entries)
	}
}

func TestNewDatabaseHasCupsAndSettings(t *testing.T) {
	dir := t.TempDir()
	conn, err := Open(filepath.Join(dir, "shop.db"), filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	cols := columnNames(t, conn, "sales")
	if !cols["cups"] || !cols["cup_price"] {
		t.Fatalf("sales columns on a new database = %v", cols)
	}
	if _, err := conn.Exec(`INSERT INTO settings (key, value) VALUES ('cup_price', '1')`); err != nil {
		t.Fatalf("settings table missing on a new database: %v", err)
	}
}

func TestMigrateV4ToV5FailureLeavesNothingHalfDone(t *testing.T) {
	dir := t.TempDir()
	conn, err := Open(filepath.Join(dir, "shop.db"), filepath.Join(dir, "backups"))
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	// Already at v5, so a second run must fail (duplicate column) ...
	if err := migrateV4ToV5(conn); err == nil {
		t.Fatal("running the step twice should fail, not silently succeed")
	}
	// ... and leave the database exactly as it was.
	var version int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil || version != 5 {
		t.Fatalf("user_version = %d (err %v), want 5", version, err)
	}
	if n := len(columnNames(t, conn, "sales")); n != 10 { // 8 original + cups + cup_price
		t.Fatalf("sales has %d columns, want 10", n)
	}
}
