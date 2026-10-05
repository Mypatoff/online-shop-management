package store

import (
	"testing"
	"time"
)

func TestCreateBilliardEntrySaved(t *testing.T) {
	s := newTestStore(t)
	minutes := int64(45)

	e, err := s.CreateBilliardEntry(1500, "Table 1", &minutes, "regular")
	if err != nil {
		t.Fatalf("create billiard entry: %v", err)
	}
	if e.ID == 0 {
		t.Fatal("entry id = 0, want nonzero")
	}
	if e.Amount != 1500 || e.TableName != "Table 1" || e.Note != "regular" {
		t.Fatalf("entry = %+v, want amount 1500, table Table 1, note regular", e)
	}
	if e.Minutes == nil || *e.Minutes != 45 {
		t.Fatalf("minutes = %v, want 45", e.Minutes)
	}
	if e.Voided() {
		t.Fatal("new entry should not be voided")
	}

	got, err := s.GetBilliardEntry(e.ID)
	if err != nil {
		t.Fatalf("get billiard entry: %v", err)
	}
	if got.ID != e.ID || got.Amount != e.Amount || got.TableName != e.TableName || got.Note != e.Note || got.CreatedAt != e.CreatedAt {
		t.Fatalf("stored entry = %+v, want %+v", got, e)
	}
	if got.Minutes == nil || *got.Minutes != 45 {
		t.Fatalf("stored minutes = %v, want 45", got.Minutes)
	}
}

func TestCreateBilliardEntryEmptyTableAndNoMinutesStoredAsNull(t *testing.T) {
	s := newTestStore(t)

	e, err := s.CreateBilliardEntry(500, "", nil, "")
	if err != nil {
		t.Fatalf("create billiard entry: %v", err)
	}
	if e.TableName != "" {
		t.Fatalf("table name = %q, want empty", e.TableName)
	}
	if e.Minutes != nil {
		t.Fatalf("minutes = %v, want nil", e.Minutes)
	}
	if e.Note != "" {
		t.Fatalf("note = %q, want empty", e.Note)
	}

	tables, err := s.RecentBilliardTables(10)
	if err != nil {
		t.Fatalf("recent tables: %v", err)
	}
	if len(tables) != 0 {
		t.Fatalf("recent tables = %v, want none (empty table name excluded)", tables)
	}
}

func TestVoidBilliardEntryTwiceFails(t *testing.T) {
	s := newTestStore(t)
	e, err := s.CreateBilliardEntry(1000, "Table 2", nil, "")
	if err != nil {
		t.Fatalf("create billiard entry: %v", err)
	}

	if err := s.VoidBilliardEntry(e.ID); err != nil {
		t.Fatalf("first void: %v", err)
	}
	got, err := s.GetBilliardEntry(e.ID)
	if err != nil {
		t.Fatalf("get billiard entry: %v", err)
	}
	if !got.Voided() {
		t.Fatal("entry should be voided after VoidBilliardEntry")
	}

	if err := s.VoidBilliardEntry(e.ID); err != ErrAlreadyVoided {
		t.Fatalf("second void: err = %v, want ErrAlreadyVoided", err)
	}
}

func TestVoidBilliardEntryNotFound(t *testing.T) {
	s := newTestStore(t)
	if err := s.VoidBilliardEntry(999); err != ErrNotFound {
		t.Fatalf("void missing entry: err = %v, want ErrNotFound", err)
	}
}

// mustInsertBilliardEntry inserts a billiard_entries row directly so
// tests can control created_at precisely.
func mustInsertBilliardEntry(t *testing.T, s *Store, amount, createdAt int64, voided bool) int64 {
	t.Helper()
	res, err := s.db.Exec(
		`INSERT INTO billiard_entries (amount, table_name, minutes, note, created_at) VALUES (?, NULL, NULL, NULL, ?)`,
		amount, createdAt,
	)
	if err != nil {
		t.Fatalf("insert billiard entry: %v", err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("insert billiard entry: id: %v", err)
	}
	if voided {
		if _, err := s.db.Exec(`UPDATE billiard_entries SET voided_at = ? WHERE id = ?`, createdAt, id); err != nil {
			t.Fatalf("void inserted billiard entry: %v", err)
		}
	}
	return id
}

func TestDailyBilliardDayBoundary(t *testing.T) {
	s := newTestStore(t)

	endOfDay1 := time.Date(2026, 6, 15, 23, 59, 59, 0, time.Local).Unix()
	startOfDay2 := time.Date(2026, 6, 16, 0, 0, 0, 0, time.Local).Unix()
	mustInsertBilliardEntry(t, s, 1000, endOfDay1, false)
	mustInsertBilliardEntry(t, s, 1000, startOfDay2, false)

	day1, err := s.DailyBilliard(time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("daily billiard day1: %v", err)
	}
	if len(day1) != 1 || day1[0].CreatedAt != endOfDay1 {
		t.Fatalf("day1 entries = %+v, want exactly the 23:59:59 entry", day1)
	}

	day2, err := s.DailyBilliard(time.Date(2026, 6, 16, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("daily billiard day2: %v", err)
	}
	if len(day2) != 1 || day2[0].CreatedAt != startOfDay2 {
		t.Fatalf("day2 entries = %+v, want exactly the 00:00:00 entry", day2)
	}
}

func TestDailyBilliardIncludesVoided(t *testing.T) {
	s := newTestStore(t)

	day := time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local).Unix()
	mustInsertBilliardEntry(t, s, 1000, day, false)
	mustInsertBilliardEntry(t, s, 2000, day, true)

	entries, err := s.DailyBilliard(time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatalf("daily billiard: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("len(entries) = %d, want 2 (voided entry must still be listed)", len(entries))
	}
	voidedCount := 0
	for _, e := range entries {
		if e.Voided() {
			voidedCount++
		}
	}
	if voidedCount != 1 {
		t.Fatalf("voided count = %d, want 1", voidedCount)
	}
}

func TestBilliardChartZeroFillsAndExcludesVoided(t *testing.T) {
	s := newTestStore(t)

	today := time.Date(2026, 6, 15, 12, 0, 0, 0, time.Local)
	mustInsertBilliardEntry(t, s, 1000, today.Unix(), false)
	mustInsertBilliardEntry(t, s, 5000, today.Unix(), true)

	buckets, err := s.BilliardChart(7, today)
	if err != nil {
		t.Fatalf("billiard chart: %v", err)
	}
	if len(buckets) != 7 {
		t.Fatalf("len(buckets) = %d, want 7", len(buckets))
	}
	last := buckets[6]
	if last.Date != "2026-06-15" || last.Revenue != 1000 {
		t.Fatalf("today's bucket = %+v, want date 2026-06-15 revenue 1000 (voided excluded)", last)
	}
	for i := 0; i < 6; i++ {
		if buckets[i].Revenue != 0 {
			t.Fatalf("bucket[%d] = %+v, want zero revenue", i, buckets[i])
		}
	}
}

func mustInsertBilliardEntryWithTable(t *testing.T, s *Store, table string, createdAt int64) int64 {
	t.Helper()
	res, err := s.db.Exec(
		`INSERT INTO billiard_entries (amount, table_name, minutes, note, created_at) VALUES (1000, ?, NULL, NULL, ?)`,
		table, createdAt,
	)
	if err != nil {
		t.Fatalf("insert billiard entry for %s: %v", table, err)
	}
	id, err := res.LastInsertId()
	if err != nil {
		t.Fatalf("insert billiard entry: id: %v", err)
	}
	return id
}

func TestRecentBilliardTablesMostRecentFirstAndLimited(t *testing.T) {
	s := newTestStore(t)
	base := time.Date(2026, 6, 15, 10, 0, 0, 0, time.Local).Unix()

	mustInsertBilliardEntryWithTable(t, s, "Table A", base)
	mustInsertBilliardEntryWithTable(t, s, "Table B", base+10)
	mustInsertBilliardEntryWithTable(t, s, "Table C", base+20)
	// Re-use "Table A" most recently so it should sort first.
	mustInsertBilliardEntryWithTable(t, s, "Table A", base+30)

	got, err := s.RecentBilliardTables(2)
	if err != nil {
		t.Fatalf("recent tables: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("len(got) = %d, want 2 (limited)", len(got))
	}
	if got[0] != "Table A" {
		t.Fatalf("got[0] = %q, want Table A (most recently used)", got[0])
	}
	if got[1] != "Table C" {
		t.Fatalf("got[1] = %q, want Table C (second most recently used)", got[1])
	}
}
