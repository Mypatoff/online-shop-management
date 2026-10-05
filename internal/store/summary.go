package store

import (
	"database/sql"
	"fmt"
	"time"
)

type LowStockItem struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	SKU               string `json:"sku"`
	Stock             int64  `json:"stock"`
	LowStockThreshold int64  `json:"low_stock_threshold"`
}

type Summary struct {
	ActiveProducts int64          `json:"active_products"`
	UnitsInStock   int64          `json:"units_in_stock"`
	StockValue     int64          `json:"stock_value"`
	TodayRevenue   int64          `json:"today_revenue"`
	TodaySaleCount int64          `json:"today_sale_count"`
	LowStock       []LowStockItem `json:"low_stock"`
}

// Summary reports shop-wide numbers as of now. "Today" is the local
// calendar day (time.Local midnight to the next midnight), computed
// with time.Date rather than a fixed 24h so it stays correct across
// daylight-saving transitions.
func (s *Store) Summary() (Summary, error) {
	sum := Summary{LowStock: []LowStockItem{}}

	row := s.db.QueryRow(`
		SELECT COUNT(*), COALESCE(SUM(stock), 0), COALESCE(SUM(stock * price), 0)
		FROM products WHERE archived = 0`)
	if err := row.Scan(&sum.ActiveProducts, &sum.UnitsInStock, &sum.StockValue); err != nil {
		return Summary{}, fmt.Errorf("summary: products: %w", err)
	}

	start, end := dayBounds(time.Now())

	row = s.db.QueryRow(`
		SELECT COALESCE(SUM(total), 0), COUNT(*)
		FROM sales WHERE voided_at IS NULL AND created_at >= ? AND created_at < ?`, start, end)
	if err := row.Scan(&sum.TodayRevenue, &sum.TodaySaleCount); err != nil {
		return Summary{}, fmt.Errorf("summary: sales: %w", err)
	}

	rows, err := s.db.Query(`
		SELECT id, name, sku, stock, low_stock_threshold
		FROM products WHERE archived = 0 AND stock <= low_stock_threshold
		ORDER BY stock, name`)
	if err != nil {
		return Summary{}, fmt.Errorf("summary: low stock: %w", err)
	}
	defer rows.Close()

	for rows.Next() {
		var item LowStockItem
		var sku sql.NullString
		if err := rows.Scan(&item.ID, &item.Name, &sku, &item.Stock, &item.LowStockThreshold); err != nil {
			return Summary{}, fmt.Errorf("summary: low stock: scan: %w", err)
		}
		item.SKU = sku.String
		sum.LowStock = append(sum.LowStock, item)
	}
	if err := rows.Err(); err != nil {
		return Summary{}, fmt.Errorf("summary: low stock: %w", err)
	}

	return sum, nil
}
