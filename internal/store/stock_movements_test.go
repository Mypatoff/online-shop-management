package store

import "testing"

func listMovements(t *testing.T, s *Store, productID int64) []StockMovement {
	t.Helper()
	movements, err := s.ListStockMovements(&productID, 1000, nil)
	if err != nil {
		t.Fatalf("list stock movements: %v", err)
	}
	return movements
}

func TestCreateProductWithStockWritesInitialMovement(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)

	movements := listMovements(t, s, p.ID)
	if len(movements) != 1 {
		t.Fatalf("len(movements) = %d, want 1", len(movements))
	}
	mv := movements[0]
	if mv.Type != "initial" || mv.Delta != 10 || mv.StockAfter != 10 {
		t.Fatalf("movement = %+v, want type=initial delta=10 stock_after=10", mv)
	}
}

func TestCreateProductWithZeroStockWritesNoMovement(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 0, 5)

	if movements := listMovements(t, s, p.ID); len(movements) != 0 {
		t.Fatalf("len(movements) = %d, want 0", len(movements))
	}
}

func TestSaleWritesOneMovement(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)

	sale, newStock, err := s.CreateSale(p.ID, 3)
	if err != nil {
		t.Fatalf("create sale: %v", err)
	}

	movements := listMovements(t, s, p.ID)
	if len(movements) != 2 { // initial + sale
		t.Fatalf("len(movements) = %d, want 2", len(movements))
	}
	mv := movements[0] // newest first
	if mv.Type != "sale" || mv.Delta != -3 || mv.StockAfter != newStock {
		t.Fatalf("movement = %+v, want type=sale delta=-3 stock_after=%d", mv, newStock)
	}
	if mv.SaleID == nil || *mv.SaleID != sale.ID {
		t.Fatalf("movement.SaleID = %v, want %d", mv.SaleID, sale.ID)
	}
}

func TestVoidWritesOneMovement(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)

	sale, _, err := s.CreateSale(p.ID, 4)
	if err != nil {
		t.Fatalf("create sale: %v", err)
	}
	if err := s.VoidSale(sale.ID); err != nil {
		t.Fatalf("void sale: %v", err)
	}

	movements := listMovements(t, s, p.ID)
	if len(movements) != 3 { // initial + sale + void
		t.Fatalf("len(movements) = %d, want 3", len(movements))
	}
	mv := movements[0] // newest first
	if mv.Type != "void" || mv.Delta != 4 || mv.StockAfter != 10 {
		t.Fatalf("movement = %+v, want type=void delta=4 stock_after=10", mv)
	}
	if mv.SaleID == nil || *mv.SaleID != sale.ID {
		t.Fatalf("movement.SaleID = %v, want %d", mv.SaleID, sale.ID)
	}
}

func TestAdjustWritesOneMovementWithReasonAndNote(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)

	newStock, err := s.AdjustStock(p.ID, 5, "restock", "delivery truck")
	if err != nil {
		t.Fatalf("adjust stock: %v", err)
	}

	movements := listMovements(t, s, p.ID)
	if len(movements) != 2 { // initial + adjust
		t.Fatalf("len(movements) = %d, want 2", len(movements))
	}
	mv := movements[0]
	if mv.Type != "adjust" || mv.Delta != 5 || mv.StockAfter != newStock {
		t.Fatalf("movement = %+v, want type=adjust delta=5 stock_after=%d", mv, newStock)
	}
	if mv.Reason != "restock" || mv.Note != "delivery truck" {
		t.Fatalf("movement reason/note = %q/%q, want restock/delivery truck", mv.Reason, mv.Note)
	}
}

func TestFailedOversellWritesNoMovement(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 5, 5)

	if _, _, err := s.CreateSale(p.ID, 6); err != ErrInsufficientStock {
		t.Fatalf("err = %v, want ErrInsufficientStock", err)
	}

	movements := listMovements(t, s, p.ID)
	if len(movements) != 1 { // just the initial movement from creation
		t.Fatalf("len(movements) = %d, want 1 (initial only)", len(movements))
	}
}

func TestFailedAdjustWritesNoMovement(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 5, 5)

	if _, err := s.AdjustStock(p.ID, -10, "recount", ""); err != ErrInsufficientStock {
		t.Fatalf("err = %v, want ErrInsufficientStock", err)
	}

	movements := listMovements(t, s, p.ID)
	if len(movements) != 1 { // just the initial movement from creation
		t.Fatalf("len(movements) = %d, want 1 (initial only)", len(movements))
	}
}

func TestSumOfMovementDeltasMatchesCurrentStock(t *testing.T) {
	s := newTestStore(t)
	p1 := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)
	p2 := mustCreateProduct(t, s, "Chips", "", 300, 0, 5)

	sale1, _, err := s.CreateSale(p1.ID, 3)
	if err != nil {
		t.Fatalf("create sale 1: %v", err)
	}
	if _, _, err := s.CreateSale(p1.ID, 2); err != nil {
		t.Fatalf("create sale 2: %v", err)
	}
	if err := s.VoidSale(sale1.ID); err != nil {
		t.Fatalf("void sale 1: %v", err)
	}
	if _, err := s.AdjustStock(p1.ID, -4, "damaged", "dropped case"); err != nil {
		t.Fatalf("adjust p1: %v", err)
	}
	if _, err := s.AdjustStock(p2.ID, 20, "restock", ""); err != nil {
		t.Fatalf("adjust p2: %v", err)
	}
	// A rejected oversell shouldn't disturb the running total either.
	if _, _, err := s.CreateSale(p2.ID, 1000); err != ErrInsufficientStock {
		t.Fatalf("err = %v, want ErrInsufficientStock", err)
	}

	for _, p := range []Product{p1, p2} {
		got, err := s.GetProduct(p.ID)
		if err != nil {
			t.Fatalf("get product %d: %v", p.ID, err)
		}

		var sum int64
		for _, mv := range listMovements(t, s, p.ID) {
			sum += mv.Delta
		}
		if sum != got.Stock {
			t.Fatalf("product %d: sum(delta) = %d, want current stock %d", p.ID, sum, got.Stock)
		}
	}
}
