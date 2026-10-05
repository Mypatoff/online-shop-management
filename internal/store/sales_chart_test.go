package store

import (
	"testing"
	"time"
)

func TestSalesChartZeroFillsEmptyDays(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 100, 5)

	today := time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local)
	mustInsertSale(t, s, p, 2, today.Unix(), false)

	buckets, err := s.SalesChart(7, today)
	if err != nil {
		t.Fatalf("sales chart: %v", err)
	}
	if len(buckets) != 7 {
		t.Fatalf("len(buckets) = %d, want 7", len(buckets))
	}
	last := buckets[6]
	if last.Date != "2026-06-15" || last.Revenue != 300 || last.Units != 2 || last.SaleCount != 1 {
		t.Fatalf("today's bucket = %+v, want date 2026-06-15 revenue 300 units 2 count 1", last)
	}
	for i := 0; i < 6; i++ {
		b := buckets[i]
		if b.Revenue != 0 || b.Units != 0 || b.SaleCount != 0 {
			t.Fatalf("bucket[%d] = %+v, want all zero (never null/missing)", i, b)
		}
		if b.Date == "" {
			t.Fatalf("bucket[%d] has empty date, want a zero-filled date", i)
		}
	}
}

func TestSalesChartBoundaryTimesLandOnDifferentDays(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 100, 5)

	endOfDay1 := time.Date(2026, 6, 14, 23, 59, 59, 0, time.Local).Unix()
	startOfDay2 := time.Date(2026, 6, 15, 0, 0, 0, 0, time.Local).Unix()
	mustInsertSale(t, s, p, 1, endOfDay1, false)
	mustInsertSale(t, s, p, 1, startOfDay2, false)

	buckets, err := s.SalesChart(2, time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("sales chart: %v", err)
	}
	if len(buckets) != 2 {
		t.Fatalf("len(buckets) = %d, want 2", len(buckets))
	}
	if buckets[0].Date != "2026-06-14" || buckets[0].SaleCount != 1 {
		t.Fatalf("day1 bucket = %+v, want the 23:59:59 sale alone", buckets[0])
	}
	if buckets[1].Date != "2026-06-15" || buckets[1].SaleCount != 1 {
		t.Fatalf("day2 bucket = %+v, want the 00:00:00 sale alone", buckets[1])
	}
}

func TestSalesChartExcludesVoided(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 100, 5)

	day := time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local)
	mustInsertSale(t, s, p, 2, day.Unix(), false)
	mustInsertSale(t, s, p, 5, day.Unix(), true)

	buckets, err := s.SalesChart(1, day)
	if err != nil {
		t.Fatalf("sales chart: %v", err)
	}
	if len(buckets) != 1 {
		t.Fatalf("len(buckets) = %d, want 1", len(buckets))
	}
	if buckets[0].Units != 2 || buckets[0].SaleCount != 1 || buckets[0].Revenue != 300 {
		t.Fatalf("bucket = %+v, want only the non-voided sale counted", buckets[0])
	}
}

func TestSalesChartSumOfLargeSalesDoesNotOverflow(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Bulk item", "", 100_000_000_000, 200_000, 5)

	today := time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local)
	mustInsertSale(t, s, p, 100_000, today.Unix(), false)
	mustInsertSale(t, s, p, 100_000, today.Unix(), false)

	buckets, err := s.SalesChart(1, today)
	if err != nil {
		t.Fatalf("sales chart: %v", err)
	}
	wantRevenue := int64(100_000_000_000) * 100_000 * 2
	if buckets[0].Revenue != wantRevenue {
		t.Fatalf("revenue = %d, want %d (no int64 overflow)", buckets[0].Revenue, wantRevenue)
	}
	if buckets[0].Revenue < 0 {
		t.Fatal("revenue went negative: overflowed int64")
	}
}

func TestSalesChartDSTRangeHasExactlyNDistinctDates(t *testing.T) {
	loc, err := time.LoadLocation("America/New_York")
	if err != nil {
		t.Skipf("no tzdata available: %v", err)
	}
	old := time.Local
	time.Local = loc
	t.Cleanup(func() { time.Local = old })

	s := newTestStore(t)

	// 2024-03-10 is the US spring-forward day; a 7-day range ending
	// 2024-03-12 spans it, and must still yield exactly 7 distinct
	// calendar dates with no duplicates or gaps.
	at := time.Date(2024, 3, 12, 12, 0, 0, 0, time.Local)
	buckets, err := s.SalesChart(7, at)
	if err != nil {
		t.Fatalf("sales chart: %v", err)
	}
	if len(buckets) != 7 {
		t.Fatalf("len(buckets) = %d, want 7", len(buckets))
	}
	seen := make(map[string]bool, 7)
	for _, b := range buckets {
		if seen[b.Date] {
			t.Fatalf("duplicate date %s in buckets: %+v", b.Date, buckets)
		}
		seen[b.Date] = true
	}
	if buckets[6].Date != "2024-03-12" {
		t.Fatalf("last bucket date = %s, want 2024-03-12", buckets[6].Date)
	}
	if buckets[0].Date != "2024-03-06" {
		t.Fatalf("first bucket date = %s, want 2024-03-06", buckets[0].Date)
	}
}
