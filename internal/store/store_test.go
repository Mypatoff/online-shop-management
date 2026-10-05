package store

import (
	"sync"
	"testing"

	"shop/internal/db"
)

// newTestStore opens a fresh in-memory database for one test.
func newTestStore(t *testing.T) *Store {
	t.Helper()
	conn, err := db.Open(":memory:", "")
	if err != nil {
		t.Fatalf("open test db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	return New(conn)
}

func mustCreateProduct(t *testing.T, s *Store, name, sku string, price, stock, threshold int64) Product {
	t.Helper()
	p, err := s.CreateProduct(name, sku, price, stock, threshold)
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	return p
}

func TestSaleLowersStock(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)

	_, newStock, err := s.CreateSale(p.ID, 3)
	if err != nil {
		t.Fatalf("create sale: %v", err)
	}
	if newStock != 7 {
		t.Fatalf("new stock = %d, want 7", newStock)
	}
	got, err := s.GetProduct(p.ID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if got.Stock != 7 {
		t.Fatalf("stored stock = %d, want 7", got.Stock)
	}
}

func TestOversellRejectedAndUnchanged(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 5, 5)

	if _, _, err := s.CreateSale(p.ID, 6); err != ErrInsufficientStock {
		t.Fatalf("err = %v, want ErrInsufficientStock", err)
	}
	got, err := s.GetProduct(p.ID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if got.Stock != 5 {
		t.Fatalf("stock changed after rejected sale: %d, want 5", got.Stock)
	}
	sales, err := s.ListSales(10)
	if err != nil {
		t.Fatalf("list sales: %v", err)
	}
	if len(sales) != 0 {
		t.Fatalf("len(sales) = %d, want 0", len(sales))
	}
}

func TestArchivedProductCantBeSold(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)
	if err := s.SetArchived(p.ID, true); err != nil {
		t.Fatalf("archive: %v", err)
	}
	if _, _, err := s.CreateSale(p.ID, 1); err != ErrArchived {
		t.Fatalf("err = %v, want ErrArchived", err)
	}
}

func TestLaterPriceEditDoesNotChangeOldSales(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)

	sale, _, err := s.CreateSale(p.ID, 1)
	if err != nil {
		t.Fatalf("create sale: %v", err)
	}
	if _, err := s.UpdateProduct(p.ID, "Cola", "", 999, 5); err != nil {
		t.Fatalf("update product: %v", err)
	}

	sales, err := s.ListSales(10)
	if err != nil {
		t.Fatalf("list sales: %v", err)
	}
	if len(sales) != 1 || sales[0].ID != sale.ID || sales[0].UnitPrice != 150 {
		t.Fatalf("sale price snapshot changed: %+v", sales[0])
	}
}

func TestVoidRestoresStockAndCantRepeat(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)

	sale, newStock, err := s.CreateSale(p.ID, 4)
	if err != nil {
		t.Fatalf("create sale: %v", err)
	}
	if newStock != 6 {
		t.Fatalf("new stock = %d, want 6", newStock)
	}

	if err := s.VoidSale(sale.ID); err != nil {
		t.Fatalf("void: %v", err)
	}
	got, err := s.GetProduct(p.ID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if got.Stock != 10 {
		t.Fatalf("stock after void = %d, want 10", got.Stock)
	}

	if err := s.VoidSale(sale.ID); err != ErrAlreadyVoided {
		t.Fatalf("second void: err = %v, want ErrAlreadyVoided", err)
	}
}

func TestAdjustBelowZeroRejected(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 5, 5)

	if _, err := s.AdjustStock(p.ID, -10, "recount", ""); err != ErrInsufficientStock {
		t.Fatalf("err = %v, want ErrInsufficientStock", err)
	}
	got, err := s.GetProduct(p.ID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if got.Stock != 5 {
		t.Fatalf("stock changed after rejected adjust: %d, want 5", got.Stock)
	}
}

func TestParallelSellsOnStockOneGiveExactlyOneSuccess(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 1, 5)

	var wg sync.WaitGroup
	results := make([]error, 10)
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, _, err := s.CreateSale(p.ID, 1)
			results[i] = err
		}(i)
	}
	wg.Wait()

	successes := 0
	for _, err := range results {
		if err == nil {
			successes++
		} else if err != ErrInsufficientStock {
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if successes != 1 {
		t.Fatalf("successes = %d, want 1", successes)
	}

	got, err := s.GetProduct(p.ID)
	if err != nil {
		t.Fatalf("get product: %v", err)
	}
	if got.Stock != 0 {
		t.Fatalf("final stock = %d, want 0", got.Stock)
	}
}

func TestEmptySKUTwiceIsAllowed(t *testing.T) {
	s := newTestStore(t)
	if _, err := s.CreateProduct("Cola", "", 150, 5, 5); err != nil {
		t.Fatalf("create #1: %v", err)
	}
	if _, err := s.CreateProduct("Sprite", "", 150, 5, 5); err != nil {
		t.Fatalf("create #2 with empty sku: %v", err)
	}
}
