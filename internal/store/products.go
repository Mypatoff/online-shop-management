package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Product is one item the shop sells.
type Product struct {
	ID                int64
	Name              string
	SKU               string // "" means no SKU (stored as NULL in the db)
	Price             int64  // smallest currency unit, e.g. cents
	Stock             int64
	LowStockThreshold int64
	Archived          bool
	CreatedAt         int64 // unix seconds, UTC
	UpdatedAt         int64
}

// Store wraps the database connection with shop-specific queries.
type Store struct {
	db *sql.DB
}

func New(db *sql.DB) *Store {
	return &Store{db: db}
}

func now() int64 {
	return time.Now().UTC().Unix()
}

// skuParam turns "" into a SQL NULL so the UNIQUE index allows any
// number of SKU-less products.
func skuParam(sku string) any {
	if sku == "" {
		return nil
	}
	return sku
}

func scanProduct(row interface{ Scan(...any) error }) (Product, error) {
	var p Product
	var sku sql.NullString
	var archived int
	if err := row.Scan(&p.ID, &p.Name, &sku, &p.Price, &p.Stock, &p.LowStockThreshold, &archived, &p.CreatedAt, &p.UpdatedAt); err != nil {
		return Product{}, err
	}
	p.SKU = sku.String
	p.Archived = archived != 0
	return p, nil
}

const productColumns = `id, name, sku, price, stock, low_stock_threshold, archived, created_at, updated_at`

func (s *Store) GetProduct(id int64) (Product, error) {
	row := s.db.QueryRow(`SELECT `+productColumns+` FROM products WHERE id = ?`, id)
	p, err := scanProduct(row)
	if errors.Is(err, sql.ErrNoRows) {
		return Product{}, ErrNotFound
	}
	if err != nil {
		return Product{}, fmt.Errorf("get product: %w", err)
	}
	return p, nil
}

// ListProducts returns products matching a case-insensitive search of
// name/SKU, optionally filtered by archived status. A nil archived
// filter returns both archived and active products.
func (s *Store) ListProducts(q string, archived *bool) ([]Product, error) {
	query := `SELECT ` + productColumns + ` FROM products WHERE 1=1`
	var args []any

	if q != "" {
		query += ` AND (LOWER(name) LIKE ? OR LOWER(COALESCE(sku, '')) LIKE ?)`
		pattern := "%" + strings.ToLower(q) + "%"
		args = append(args, pattern, pattern)
	}
	if archived != nil {
		query += ` AND archived = ?`
		args = append(args, boolToInt(*archived))
	}
	query += ` ORDER BY name`

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list products: %w", err)
	}
	defer rows.Close()

	var products []Product
	for rows.Next() {
		p, err := scanProduct(rows)
		if err != nil {
			return nil, fmt.Errorf("list products: scan: %w", err)
		}
		products = append(products, p)
	}
	return products, rows.Err()
}

func (s *Store) CreateProduct(name, sku string, price, stock, threshold int64) (Product, error) {
	t := now()

	tx, err := s.db.Begin()
	if err != nil {
		return Product{}, fmt.Errorf("create product: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	res, err := tx.Exec(
		`INSERT INTO products (name, sku, price, stock, low_stock_threshold, archived, created_at, updated_at)
		 VALUES (?, ?, ?, ?, ?, 0, ?, ?)`,
		name, skuParam(sku), price, stock, threshold, t, t,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Product{}, ErrDuplicateSKU
		}
		return Product{}, fmt.Errorf("create product: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return Product{}, fmt.Errorf("create product: %w", err)
	}

	if stock > 0 {
		if err := recordMovement(tx, id, "initial", stock, stock, "", "", nil, t); err != nil {
			return Product{}, fmt.Errorf("create product: %w", err)
		}
	}

	if err := tx.Commit(); err != nil {
		return Product{}, fmt.Errorf("create product: commit: %w", err)
	}
	return s.GetProduct(id)
}

// UpdateProduct changes name, SKU and price/threshold. Stock is never
// touched here; use AdjustStock for that.
func (s *Store) UpdateProduct(id int64, name, sku string, price, threshold int64) (Product, error) {
	res, err := s.db.Exec(
		`UPDATE products SET name = ?, sku = ?, price = ?, low_stock_threshold = ?, updated_at = ? WHERE id = ?`,
		name, skuParam(sku), price, threshold, now(), id,
	)
	if err != nil {
		if isUniqueViolation(err) {
			return Product{}, ErrDuplicateSKU
		}
		return Product{}, fmt.Errorf("update product: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return Product{}, ErrNotFound
	}
	return s.GetProduct(id)
}

// AdjustStock changes a product's stock by delta (positive or
// negative) and returns the new stock. The result may never go below
// zero. reason is required (an enum enforced by the API layer); note
// is optional.
func (s *Store) AdjustStock(id, delta int64, reason, note string) (newStock int64, err error) {
	tx, err := s.db.Begin()
	if err != nil {
		return 0, fmt.Errorf("adjust stock: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	var stock int64
	if err := tx.QueryRow(`SELECT stock FROM products WHERE id = ?`, id).Scan(&stock); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return 0, ErrNotFound
		}
		return 0, fmt.Errorf("adjust stock: lookup: %w", err)
	}

	newStock = stock + delta
	if newStock < 0 {
		return 0, ErrInsufficientStock
	}

	t := now()
	if _, err := tx.Exec(`UPDATE products SET stock = ?, updated_at = ? WHERE id = ?`, newStock, t, id); err != nil {
		return 0, fmt.Errorf("adjust stock: update: %w", err)
	}
	if err := recordMovement(tx, id, "adjust", delta, newStock, reason, note, nil, t); err != nil {
		return 0, fmt.Errorf("adjust stock: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return 0, fmt.Errorf("adjust stock: commit: %w", err)
	}
	return newStock, nil
}

func (s *Store) SetArchived(id int64, archived bool) error {
	res, err := s.db.Exec(`UPDATE products SET archived = ?, updated_at = ? WHERE id = ?`, boolToInt(archived), now(), id)
	if err != nil {
		return fmt.Errorf("set archived: %w", err)
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// isUniqueViolation reports whether err came from a UNIQUE constraint
// (e.g. a duplicate SKU). modernc.org/sqlite doesn't export a typed
// error for this, so we match on the driver's message text.
func isUniqueViolation(err error) bool {
	return err != nil && strings.Contains(err.Error(), "UNIQUE constraint failed")
}
