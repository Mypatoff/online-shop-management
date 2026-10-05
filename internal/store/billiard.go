package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// BilliardEntry is one payment received for table time: no stock or
// product is involved.
type BilliardEntry struct {
	ID        int64
	Amount    int64
	TableName string // "" means not recorded (stored as NULL)
	Minutes   *int64 // nil means not recorded (stored as NULL)
	Note      string // "" if not given
	CreatedAt int64
	VoidedAt  *int64
}

func (e BilliardEntry) Voided() bool {
	return e.VoidedAt != nil
}

const billiardColumns = `id, amount, table_name, minutes, note, created_at, voided_at`

func scanBilliardEntry(row interface{ Scan(...any) error }) (BilliardEntry, error) {
	var e BilliardEntry
	var tableName, note sql.NullString
	var minutes, voidedAt sql.NullInt64
	if err := row.Scan(&e.ID, &e.Amount, &tableName, &minutes, &note, &e.CreatedAt, &voidedAt); err != nil {
		return BilliardEntry{}, err
	}
	e.TableName = tableName.String
	e.Note = note.String
	if minutes.Valid {
		m := minutes.Int64
		e.Minutes = &m
	}
	if voidedAt.Valid {
		v := voidedAt.Int64
		e.VoidedAt = &v
	}
	return e, nil
}

func minutesParam(m *int64) any {
	if m == nil {
		return nil
	}
	return *m
}

// CreateBilliardEntry records a payment for table time. table and
// note are stored as NULL when empty; minutes is stored as NULL when
// nil.
func (s *Store) CreateBilliardEntry(amount int64, table string, minutes *int64, note string) (BilliardEntry, error) {
	res, err := s.db.Exec(
		`INSERT INTO billiard_entries (amount, table_name, minutes, note, created_at) VALUES (?, ?, ?, ?, ?)`,
		amount, nullStr(table), minutesParam(minutes), nullStr(note), now(),
	)
	if err != nil {
		return BilliardEntry{}, fmt.Errorf("create billiard entry: %w", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		return BilliardEntry{}, fmt.Errorf("create billiard entry: %w", err)
	}
	return s.GetBilliardEntry(id)
}

func (s *Store) GetBilliardEntry(id int64) (BilliardEntry, error) {
	row := s.db.QueryRow(`SELECT `+billiardColumns+` FROM billiard_entries WHERE id = ?`, id)
	e, err := scanBilliardEntry(row)
	if errors.Is(err, sql.ErrNoRows) {
		return BilliardEntry{}, ErrNotFound
	}
	if err != nil {
		return BilliardEntry{}, fmt.Errorf("get billiard entry: %w", err)
	}
	return e, nil
}

// VoidBilliardEntry marks an entry voided. There's no stock to
// restore, unlike VoidSale.
func (s *Store) VoidBilliardEntry(id int64) error {
	var voidedAt sql.NullInt64
	row := s.db.QueryRow(`SELECT voided_at FROM billiard_entries WHERE id = ?`, id)
	if err := row.Scan(&voidedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return ErrNotFound
		}
		return fmt.Errorf("void billiard entry: lookup: %w", err)
	}
	if voidedAt.Valid {
		return ErrAlreadyVoided
	}
	if _, err := s.db.Exec(`UPDATE billiard_entries SET voided_at = ? WHERE id = ?`, now(), id); err != nil {
		return fmt.Errorf("void billiard entry: %w", err)
	}
	return nil
}

// DailyBilliard returns every billiard entry (including voided) that
// falls in the local calendar day containing at, oldest first.
func (s *Store) DailyBilliard(at time.Time) ([]BilliardEntry, error) {
	start, end := dayBounds(at)

	rows, err := s.db.Query(
		`SELECT `+billiardColumns+` FROM billiard_entries WHERE created_at >= ? AND created_at < ? ORDER BY id ASC`,
		start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("daily billiard: %w", err)
	}
	defer rows.Close()

	entries := []BilliardEntry{}
	for rows.Next() {
		e, err := scanBilliardEntry(rows)
		if err != nil {
			return nil, fmt.Errorf("daily billiard: scan: %w", err)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// RecentBilliardTables returns up to limit distinct non-empty table
// names, most-recently-used first, for suggesting table names on the
// entry form.
func (s *Store) RecentBilliardTables(limit int) ([]string, error) {
	rows, err := s.db.Query(
		`SELECT table_name, MAX(created_at) AS last_used, MAX(id) AS last_id
		 FROM billiard_entries
		 WHERE table_name IS NOT NULL AND table_name != ''
		 GROUP BY table_name
		 ORDER BY last_used DESC, last_id DESC
		 LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("recent billiard tables: %w", err)
	}
	defer rows.Close()

	tables := []string{}
	for rows.Next() {
		var name string
		var lastUsed, lastID int64
		if err := rows.Scan(&name, &lastUsed, &lastID); err != nil {
			return nil, fmt.Errorf("recent billiard tables: scan: %w", err)
		}
		tables = append(tables, name)
	}
	return tables, rows.Err()
}

// BilliardChartBucket is one local calendar day's non-voided billiard
// revenue for the daily sales chart. Days with no entries come back
// as zero, never omitted.
type BilliardChartBucket struct {
	Date    string
	Revenue int64
}

// BilliardChart returns exactly days buckets, oldest first, ending
// with the local calendar day containing at - mirroring SalesChart's
// one-query-then-bucket-in-Go approach.
func (s *Store) BilliardChart(days int, at time.Time) ([]BilliardChartBucket, error) {
	y, m, d := at.Date()
	today := time.Date(y, m, d, 0, 0, 0, 0, time.Local)
	firstDay := today.AddDate(0, 0, -(days - 1))

	start, _ := dayBounds(firstDay)
	_, end := dayBounds(today)

	rows, err := s.db.Query(
		`SELECT created_at, amount FROM billiard_entries WHERE voided_at IS NULL AND created_at >= ? AND created_at < ?`,
		start, end,
	)
	if err != nil {
		return nil, fmt.Errorf("billiard chart: %w", err)
	}
	defer rows.Close()

	byDate := make(map[string]int64, days)
	for rows.Next() {
		var createdAt, amount int64
		if err := rows.Scan(&createdAt, &amount); err != nil {
			return nil, fmt.Errorf("billiard chart: scan: %w", err)
		}
		date := time.Unix(createdAt, 0).In(time.Local).Format(chartDateLayout)
		byDate[date] += amount
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("billiard chart: %w", err)
	}

	buckets := make([]BilliardChartBucket, days)
	for i := 0; i < days; i++ {
		date := firstDay.AddDate(0, 0, i).Format(chartDateLayout)
		buckets[i] = BilliardChartBucket{Date: date, Revenue: byDate[date]}
	}
	return buckets, nil
}
