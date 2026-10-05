package store

import (
	"database/sql"
	"fmt"
)

// StockMovement is one recorded change to a product's stock.
type StockMovement struct {
	ID          int64
	ProductID   int64
	ProductName string
	Type        string // "initial", "sale", "void", or "adjust"
	Delta       int64
	StockAfter  int64
	Reason      string // "" unless Type == "adjust"
	Note        string // "" if not given
	SaleID      *int64 // non-nil for "sale" and "void"
	CreatedAt   int64
}

// nullStr turns "" into a SQL NULL, so optional text columns don't
// store empty strings.
func nullStr(s string) any {
	if s == "" {
		return nil
	}
	return s
}

// recordMovement inserts one stock_movements row within tx, so it
// always lands in the same transaction as the stock change it logs.
func recordMovement(tx *sql.Tx, productID int64, movementType string, delta, stockAfter int64, reason, note string, saleID *int64, createdAt int64) error {
	var saleIDArg any
	if saleID != nil {
		saleIDArg = *saleID
	}
	_, err := tx.Exec(
		`INSERT INTO stock_movements (product_id, type, delta, stock_after, reason, note, sale_id, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		productID, movementType, delta, stockAfter, nullStr(reason), nullStr(note), saleIDArg, createdAt,
	)
	if err != nil {
		return fmt.Errorf("record movement: %w", err)
	}
	return nil
}

const stockMovementColumns = `m.id, m.product_id, p.name, m.type, m.delta, m.stock_after, m.reason, m.note, m.sale_id, m.created_at`

func scanStockMovement(row interface{ Scan(...any) error }) (StockMovement, error) {
	var mv StockMovement
	var reason, note sql.NullString
	var saleID sql.NullInt64
	if err := row.Scan(
		&mv.ID, &mv.ProductID, &mv.ProductName, &mv.Type, &mv.Delta, &mv.StockAfter,
		&reason, &note, &saleID, &mv.CreatedAt,
	); err != nil {
		return StockMovement{}, err
	}
	mv.Reason = reason.String
	mv.Note = note.String
	if saleID.Valid {
		id := saleID.Int64
		mv.SaleID = &id
	}
	return mv, nil
}

// ListStockMovements returns stock movements newest-first, joined with
// the current product name. productID filters to one product; beforeID
// pages backward (strictly older than that movement id). Callers are
// expected to have already clamped limit to a sane range.
func (s *Store) ListStockMovements(productID *int64, limit int, beforeID *int64) ([]StockMovement, error) {
	query := `SELECT ` + stockMovementColumns + ` FROM stock_movements m JOIN products p ON p.id = m.product_id WHERE 1=1`
	var args []any

	if productID != nil {
		query += ` AND m.product_id = ?`
		args = append(args, *productID)
	}
	if beforeID != nil {
		query += ` AND m.id < ?`
		args = append(args, *beforeID)
	}
	query += ` ORDER BY m.id DESC LIMIT ?`
	args = append(args, limit)

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("list stock movements: %w", err)
	}
	defer rows.Close()

	var out []StockMovement
	for rows.Next() {
		mv, err := scanStockMovement(rows)
		if err != nil {
			return nil, fmt.Errorf("list stock movements: scan: %w", err)
		}
		out = append(out, mv)
	}
	return out, rows.Err()
}
