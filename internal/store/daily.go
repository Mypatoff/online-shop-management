package store

import (
	"fmt"
	"time"
)

// dayBounds returns the start (inclusive) and end (exclusive) unix
// timestamps of the local calendar day containing t. It's computed
// with time.Date rather than t.Add(24*time.Hour) so it stays correct
// across daylight-saving transitions, where a local day can be 23 or
// 25 hours long.
func dayBounds(t time.Time) (start, end int64) {
	y, m, d := t.Date()
	s := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	return s.Unix(), s.AddDate(0, 0, 1).Unix()
}

// DailySales returns every sale (including voided ones) that falls in
// the local calendar day containing at, oldest first. At most 5000
// sales are returned.
func (s *Store) DailySales(at time.Time) ([]Sale, error) {
	start, end := dayBounds(at)

	rows, err := s.db.Query(
		`SELECT `+saleColumns+` FROM sales WHERE created_at >= ? AND created_at < ? ORDER BY id ASC LIMIT 5000`,
		start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("daily sales: %w", err)
	}
	defer rows.Close()

	sales := []Sale{}
	for rows.Next() {
		sale, err := scanSale(rows)
		if err != nil {
			return nil, fmt.Errorf("daily sales: scan: %w", err)
		}
		sales = append(sales, sale)
	}
	return sales, rows.Err()
}
