package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
)

const cupPriceKey = "cup_price"

// queryer is satisfied by both *sql.DB and *sql.Tx.
type queryer interface {
	QueryRow(query string, args ...any) *sql.Row
}

// cupPriceIn reads the cup price (0 until one is set) through q.
func cupPriceIn(q queryer) (int64, error) {
	var v string
	err := q.QueryRow(`SELECT value FROM settings WHERE key = ?`, cupPriceKey).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return 0, nil
	}
	if err != nil {
		return 0, fmt.Errorf("read cup price: %w", err)
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0, fmt.Errorf("read cup price: stored value %q is not a price", v)
	}
	return n, nil
}

// CupPrice is what one cup (stakan) costs when sold with a drink, in the same minor units as
// product prices. 0 until the shop sets one.
func (s *Store) CupPrice() (int64, error) {
	return cupPriceIn(s.db)
}

// SetCupPrice changes the price new sales charge per cup. Sales already made keep the price
// they were made at.
func (s *Store) SetCupPrice(price int64) error {
	if price < 0 {
		return fmt.Errorf("set cup price: price must not be negative")
	}
	_, err := s.db.Exec(
		`INSERT INTO settings (key, value) VALUES (?, ?)
		 ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		cupPriceKey, strconv.FormatInt(price, 10),
	)
	if err != nil {
		return fmt.Errorf("set cup price: %w", err)
	}
	return nil
}
