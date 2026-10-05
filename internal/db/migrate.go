package db

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"time"
)

// currentVersion is tracked via PRAGMA user_version. Any database that
// already had its tables (products/sales) before versioning existed is
// treated as version 1; a brand-new database starts at version 0 and is
// created directly at currentVersion.
const currentVersion = 2

const stockMovementsSchema = `
CREATE TABLE IF NOT EXISTS stock_movements (
	id          INTEGER PRIMARY KEY AUTOINCREMENT,
	product_id  INTEGER NOT NULL REFERENCES products(id),
	type        TEXT NOT NULL CHECK (type IN ('initial', 'sale', 'void', 'adjust')),
	delta       INTEGER NOT NULL,
	stock_after INTEGER NOT NULL,
	reason      TEXT,
	note        TEXT,
	sale_id     INTEGER REFERENCES sales(id),
	created_at  INTEGER NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_stock_movements_product_created ON stock_movements(product_id, created_at);
`

// execer is satisfied by both *sql.DB and *sql.Tx.
type execer interface {
	Exec(query string, args ...any) (sql.Result, error)
}

func setUserVersion(e execer, v int) error {
	// PRAGMAs don't accept bound parameters; v is always currentVersion, a
	// compile-time constant, so this is safe to format directly.
	_, err := e.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, v))
	return err
}

// migrate brings conn's schema up to currentVersion. hadTablesBefore
// tells it whether products/sales already existed before baseSchema
// ran in Open, which is how a never-versioned existing database (user_
// version 0) is told apart from a genuinely brand-new one.
func migrate(conn *sql.DB, backupDir string, hadTablesBefore bool) error {
	var version int
	if err := conn.QueryRow(`PRAGMA user_version`).Scan(&version); err != nil {
		return fmt.Errorf("read user_version: %w", err)
	}

	if version >= currentVersion {
		return nil
	}

	if version == 0 {
		if !hadTablesBefore {
			// Brand-new database: nothing to migrate or back up, go
			// straight to the current schema.
			if _, err := conn.Exec(stockMovementsSchema); err != nil {
				return fmt.Errorf("create stock_movements: %w", err)
			}
			return setUserVersion(conn, currentVersion)
		}
		version = 1
	}

	if version == 1 {
		return migrateV1ToV2(conn, backupDir)
	}

	return fmt.Errorf("database user_version %d is newer than this program supports", version)
}

// migrateV1ToV2 adds stock_movements and backfills one 'initial' row per
// existing product. It takes a premigration safety backup first and
// aborts without touching the schema if that backup fails.
func migrateV1ToV2(conn *sql.DB, backupDir string) error {
	premigrationPath := filepath.Join(backupDir, fmt.Sprintf("shop-premigration-%s.db", time.Now().Format("20060102-150405")))
	if err := Backup(conn, premigrationPath); err != nil {
		return fmt.Errorf("premigration backup failed, aborting startup: %w", err)
	}

	tx, err := conn.Begin()
	if err != nil {
		return fmt.Errorf("begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	if _, err := tx.Exec(stockMovementsSchema); err != nil {
		return fmt.Errorf("create stock_movements: %w", err)
	}

	rows, err := tx.Query(`SELECT id, stock, created_at FROM products`)
	if err != nil {
		return fmt.Errorf("list products for backfill: %w", err)
	}
	type product struct {
		id, stock, createdAt int64
	}
	var products []product
	for rows.Next() {
		var p product
		if err := rows.Scan(&p.id, &p.stock, &p.createdAt); err != nil {
			rows.Close()
			return fmt.Errorf("scan product for backfill: %w", err)
		}
		products = append(products, p)
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return fmt.Errorf("list products for backfill: %w", err)
	}
	rows.Close()

	for _, p := range products {
		if _, err := tx.Exec(
			`INSERT INTO stock_movements (product_id, type, delta, stock_after, reason, note, sale_id, created_at)
			 VALUES (?, 'initial', ?, ?, NULL, 'Before history tracking', NULL, ?)`,
			p.id, p.stock, p.stock, p.createdAt,
		); err != nil {
			return fmt.Errorf("backfill movement for product %d: %w", p.id, err)
		}
	}

	if err := setUserVersion(tx, currentVersion); err != nil {
		return fmt.Errorf("set user_version: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit: %w", err)
	}
	return nil
}
