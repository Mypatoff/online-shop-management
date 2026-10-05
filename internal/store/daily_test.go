package store

import (
	"testing"
	"time"
)

// mustInsertSale inserts a sale row directly (bypassing CreateSale) so
// tests can control created_at precisely, and optionally mark it
// voided.
func mustInsertSale(t *testing.T, s *Store, p Product, quantity, createdAt int64, voided bool) int64 {
	t.Helper()
	total := p.Price * quantity
	res, err := s.db.Exec(
		`INSERT INTO sales (product_id, product_name, quantity, unit_price, total, created_at) VALUES (?, ?, ?, ?, ?, ?)`,
		p.ID, p.Name, quantity, p.Price, total, createdAt,
	)
	if err != nil {
		t.Fatalf("insert sale: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("insert sale: id: %v", err)
	}
	if voided {
		if _, err := s.db.Exec(`UPDATE sales SET voided_at = ? WHERE id = ?`, createdAt, id); err != nil {
			t.Fatalf("void inserted sale: %v", err)
		}
	}
	return id
}

func TestDayBoundsNormalDay(t *testing.T) {
	start, end := dayBounds(time.Date(2026, 6, 15, 13, 45, 0, 0, time.Local))

	wantStart := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local).Unix()
	wantEnd := time.Date(2026, 6, 16, 0, 0, 0, 0, time.Local).Unix()

	if start != wantStart {
		t.Fatalf("start = %d, want %d", start, wantStart)
	}
	if end != wantEnd {
		t.Fatalf("end = %d, want %d", end, wantEnd)
	}
	if end-start != 86400 {
		t.Fatalf("day length = %d seconds, want 86400", end-start)
	}
}

func TestDayBoundsDSTChangeDay(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("no tzdata available: %v", err)
	}
	old := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = old })

	// 2024-03-10: spring forward, clocks skip 2am-3am -> 23-hour day.
	start, end := dayBounds(time.Date(2024, 3, 10, 12, 0, 0, 0, time.Local))
	if got := end - start; got != 23*3600 {
		t.Fatalf("spring-forward day length = %d seconds, want %d", got, 23*3600)
	}

	// 2024-11-03: fall back, 1am-2am repeats -> 25-hour day.
	start, end = dayBounds(time.Date(2024, 11, 3, 12, 0, 0, 0, time.Local))
	if got := end - start; got != 25*3600 {
		t.Fatalf("fall-back day length = %d seconds, want %d", got, 25*3600)
	}
}

func TestDailySalesDayBoundary(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 100, 5)

	endOfDay1 := time.Date(2026, 6, 15, 23, 59, 59, 0, time.Local).Unix()
	startOfDay2 := time.Date(2026, 6, 16, 0, 0, 0, 0, time.Local).Unix()
	mustInsertSale(t, s, p, 1, endOfDay1, false)
	mustInsertSale(t, s, p, 1, startOfDay2, false)

	day1, err := s.DailySales(time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("daily sales day1: %v", err)
	}
	if len(day1) != 1 || day1[0].CreatedAt != endOfDay1 {
		t.Fatalf("day1 sales = %+v, want exactly the 23:59:59 sale", day1)
	}

	day2, err := s.DailySales(time.Date(2026, 6, 16, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("daily sales day2: %v", err)
	}
	if len(day2) != 1 || day2[0].CreatedAt != startOfDay2 {
		t.Fatalf("day2 sales = %+v, want exactly the 00:00:00 sale", day2)
	}
}

func TestDailySalesEmptyDayReturnsEmptyNotNilSlice(t *testing.T) {
	s := newTestStore(t)

	sales, err := s.DailySales(time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("daily sales: %v", err)
	}
	if sales == nil {
		t.Fatal("sales = nil, want empty non-nil slice")
	}
	if len(sales) != 0 {
		t.Fatalf("len(sales) = %d, want 0", len(sales))
	}
}

func TestDailySalesIncludesVoided(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 100, 5)

	day := time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local).Unix()
	mustInsertSale(t, s, p, 2, day, false)
	mustInsertSale(t, s, p, 3, day, true)

	sales, err := s.DailySales(time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("daily sales: %v", err)
	}
	if len(sales) != 2 {
		t.Fatalf("len(sales) = %d, want 2 (both voided and non-voided listed)", len(sales))
	}
	voidedCount := 0
	for _, sale := range sales {
		if sale.Voided() {
			voidedCount++
		}
	}
	if voidedCount != 1 {
		t.Fatalf("voided count = %d, want 1", voidedCount)
	}
}
