package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// Sale is one completed (or later voided) sale of a product.
type Sale struct {
	ID          int64
	ProductID   int64
	ProductName string
	Quantity    int64
	UnitPrice   int64
	Cups        int64 // cups (stakan) sold with the product; priced at CupPrice each
	CupPrice    int64 // what one cup cost when this sale was made
	Total       int64 // Quantity*UnitPrice + Cups*CupPrice
	CreatedAt   int64
	VoidedAt    *int64
}

func (s Sale) Voided() bool {
	return s.VoidedAt != nil
}

const saleColumns = `id, product_id, product_name, quantity, unit_price, cups, cup_price, total, created_at, voided_at`

func scanSale(row interface{ Scan(...any) error }) (Sale, error) {
	var sale Sale
	var voidedAt sql.NullInt64
	if err := row.Scan(&sale.ID, &sale.ProductID, &sale.ProductName, &sale.Quantity, &sale.UnitPrice, &sale.Cups, &sale.CupPrice, &sale.Total, &sale.CreatedAt, &voidedAt); err != nil {
		return Sale{}, err
	}
	if voidedAt.Valid {
		sale.VoidedAt = &voidedAt.Int64
	}
	return sale, nil
}

// CreateSale sells quantity units of productID, plus cups cups at the current cup price: it
// checks the product exists and isn't archived, checks there's enough stock, decrements
// stock, and records the sale with a price snapshot (the product's and the cup's) — all in
// one transaction. Cups have no stock; they only add to the total.
func (s *Store) CreateSale(productID, quantity, cups int64) (Sale, int64, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Sale{}, 0, fmt.Errorf("create sale: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	var name string
	var price, stock int64
	var archived int
	row := tx.QueryRow(`SELECT name, price, stock, archived FROM products WHERE id = ?`, productID)
	if err := row.Scan(&name, &price, &stock, &archived); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Sale{}, 0, ErrNotFound
		}
		return Sale{}, 0, fmt.Errorf("create sale: lookup: %w", err)
	}
	if archived != 0 {
		return Sale{}, 0, ErrArchived
	}
	if quantity > stock {
		return Sale{}, 0, ErrInsufficientStock
	}

	var cupPrice int64
	if cups > 0 {
		cupPrice, err = cupPriceIn(tx)
		if err != nil {
			return Sale{}, 0, fmt.Errorf("create sale: %w", err)
		}
	}

	newStock := stock - quantity
	t := now()
	if _, err := tx.Exec(`UPDATE products SET stock = ?, updated_at = ? WHERE id = ?`, newStock, t, productID); err != nil {
		return Sale{}, 0, fmt.Errorf("create sale: update stock: %w", err)
	}

	total := price*quantity + cups*cupPrice
	res, err := tx.Exec(
		`INSERT INTO sales (product_id, product_name, quantity, unit_price, cups, cup_price, total, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		productID, name, quantity, price, cups, cupPrice, total, t,
	)
	if err != nil {
		return Sale{}, 0, fmt.Errorf("create sale: insert: %w", err)
	}
	saleID, err := res.LastInsertId()
	if err != nil {
		return Sale{}, 0, fmt.Errorf("create sale: insert id: %w", err)
	}

	if err := recordMovement(tx, productID, "sale", -quantity, newStock, "", "", &saleID, t); err != nil {
		return Sale{}, 0, fmt.Errorf("create sale: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return Sale{}, 0, fmt.Errorf("create sale: commit: %w", err)
	}

	sale := Sale{ID: saleID, ProductID: productID, ProductName: name, Quantity: quantity, UnitPrice: price, Cups: cups, CupPrice: cupPrice, Total: total, CreatedAt: t}
	return sale, newStock, nil
}

// VoidSale marks a sale voided and restores the stock it consumed.
func (s *Store) VoidSale(id int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("void sale: begin: %w", err)
	}
	defer tx.Rollback() //nolint:errcheck // no-op if committed

	var productID, quantity int64
	var voidedAt sql.NullInt64
	row := tx.QueryRow(`SELECT product_id, quantity, voided_at FROM sales WHERE id = ?`, id)
	if err := row.Scan(&productID, &quantity, &voidedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("void sale: lookup: %w", err)
	}
	if voidedAt.Valid {
		return ErrAlreadyVoided
	}

	var stock int64
	if err := tx.QueryRow(`SELECT stock FROM products WHERE id = ?`, productID).Scan(&stock); err != nil {
		return fmt.Errorf("void sale: lookup product stock: %w", err)
	}
	newStock := stock + quantity

	t := now()
	if _, err := tx.Exec(`UPDATE sales SET voided_at = ? WHERE id = ?`, t, id); err != nil {
		return fmt.Errorf("void sale: update sale: %w", err)
	}
	if _, err := tx.Exec(`UPDATE products SET stock = ?, updated_at = ? WHERE id = ?`, newStock, t, productID); err != nil {
		return fmt.Errorf("void sale: restore stock: %w", err)
	}
	if err := recordMovement(tx, productID, "void", quantity, newStock, "", "", &id, t); err != nil {
		return fmt.Errorf("void sale: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("void sale: commit: %w", err)
	}
	return nil
}

// ListSales returns the most recent sales, newest first, including
// voided ones.
func (s *Store) ListSales(limit int) ([]Sale, error) {
	rows, err := s.db.Query(`SELECT `+saleColumns+` FROM sales ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("list sales: %w", err)
	}
	defer rows.Close()

	var sales []Sale
	for rows.Next() {
		sale, err := scanSale(rows)
		if err != nil {
			return nil, fmt.Errorf("list sales: scan: %w", err)
		}
		sales = append(sales, sale)
	}
	return sales, rows.Err()
}
