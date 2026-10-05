package store

import (
	"fmt"
	"time"
)

const chartDateLayout = "2006-01-02"

// ChartBucket is one local calendar day's non-voided sales totals for
// the daily sales chart. Days with no sales come back as zeros, never
// omitted.
type ChartBucket struct {
	Date      string
	Revenue   int64
	Units     int64
	SaleCount int64
}

// SalesChart returns exactly days buckets, oldest first, ending with
// the local calendar day containing at. It queries every non-voided
// sale in that whole range in one query, then buckets them in Go by
// local calendar date, so the result is correct across DST changes.
func (s *Store) SalesChart(days int, at time.Time) ([]ChartBucket, error) {
	y, m, d := at.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	firstDay := today.AddDate(0, 0, -(days - 1))

	start, _ := dayBounds(firstDay)
	_, end := dayBounds(today)

	rows, err := s.db.Query(
		`SELECT created_at, total, quantity FROM sales WHERE voided_at IS NULL AND created_at >= ? AND created_at < ?`,
		start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("sales chart: %w", err)
	}
	defer rows.Close()

	byDate := make(map[string]*ChartBucket, days)
	for rows.Next() {
		var createdAt, total, quantity int64
		if err := rows.Scan(&createdAt, &total, &quantity); err != nil {
			return nil, fmt.Errorf("sales chart: scan: %w", err)
		}
		date := time.Unix(createdAt, 0).In(time.Local).Format(chartDateLayout)
		b, ok := byDate[date]
		if !ok {
			b = &ChartBucket{Date: date}
			byDate[date] = b
		}
		b.Revenue += total
		b.Units += quantity
		b.SaleCount++
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("sales chart: %w", err)
	}

	buckets := make([]ChartBucket, days)
	for i := 0; i < days; i++ {
		date := firstDay.AddDate(0, 0, i).Format(chartDateLayout)
		if b, ok := byDate[date]; ok {
			buckets[i] = *b
		} else {
			buckets[i] = ChartBucket{Date: date}
		}
	}
	return buckets, nil
}
