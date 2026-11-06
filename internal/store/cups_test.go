package store

import "testing"

func TestCupPriceDefaultsToZeroAndCanBeSet(t *testing.T) {
	s := newTestStore(t)
	if p, err := s.CupPrice(); err != nil || p != 0 {
		t.Fatalf("CupPrice = %d, %v; want 0, nil before any is set", p, err)
	}
	if err := s.SetCupPrice(500); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCupPrice(700); err != nil { // overwriting must work, not fail on the key
		t.Fatal(err)
	}
	if p, _ := s.CupPrice(); p != 700 {
		t.Fatalf("CupPrice = %d, want 700", p)
	}
	if err := s.SetCupPrice(-1); err == nil {
		t.Fatal("a negative cup price must be refused")
	}
}

func TestSaleWithCupsAddsThemToTheTotalNotToStock(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)
	if err := s.SetCupPrice(20); err != nil {
		t.Fatal(err)
	}

	sale, stock, err := s.CreateSale(p.ID, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if sale.Total != 2*150+3*20 || sale.Cups != 3 || sale.CupPrice != 20 || sale.UnitPrice != 150 {
		t.Fatalf("sale = %+v, want total 360 with 3 cups at 20", sale)
	}
	if stock != 8 {
		t.Fatalf("stock = %d, want 8: cups must not touch the product's stock", stock)
	}

	// It must be what was stored, not just what was returned.
	sales, err := s.ListSales(10)
	if err != nil || len(sales) != 1 {
		t.Fatalf("ListSales = %v, %v", sales, err)
	}
	if got := sales[0]; got.Total != 360 || got.Cups != 3 || got.CupPrice != 20 {
		t.Fatalf("stored sale = %+v", got)
	}
}

func TestSaleWithoutCupsIsUnchanged(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)
	if err := s.SetCupPrice(20); err != nil {
		t.Fatal(err)
	}
	sale, _, err := s.CreateSale(p.ID, 2, 0)
	if err != nil {
		t.Fatal(err)
	}
	if sale.Total != 300 || sale.Cups != 0 || sale.CupPrice != 0 {
		t.Fatalf("sale = %+v, want a plain 300 with no cups", sale)
	}
}

func TestCupsWithNoPriceSetAreCountedForFree(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)
	sale, _, err := s.CreateSale(p.ID, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	if sale.Total != 150 || sale.Cups != 2 || sale.CupPrice != 0 {
		t.Fatalf("sale = %+v, want total 150 with 2 free cups", sale)
	}
}

func TestChangingTheCupPriceDoesNotRewriteOldSales(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)
	if err := s.SetCupPrice(20); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateSale(p.ID, 1, 1); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCupPrice(999); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateSale(p.ID, 1, 1); err != nil {
		t.Fatal(err)
	}
	sales, err := s.ListSales(10) // newest first
	if err != nil {
		t.Fatal(err)
	}
	if sales[1].CupPrice != 20 || sales[1].Total != 170 {
		t.Fatalf("old sale = %+v, want cup price 20 and total 170", sales[1])
	}
	if sales[0].CupPrice != 999 || sales[0].Total != 1149 {
		t.Fatalf("new sale = %+v, want cup price 999 and total 1149", sales[0])
	}
}

func TestFailedSaleLeavesNoCupsBehind(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 1, 5)
	if err := s.SetCupPrice(20); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateSale(p.ID, 5, 2); err != ErrInsufficientStock {
		t.Fatalf("err = %v, want ErrInsufficientStock", err)
	}
	if sales, _ := s.ListSales(10); len(sales) != 0 {
		t.Fatalf("a refused sale was recorded: %+v", sales)
	}
}

func TestVoidingASaleVoidsItsCupMoneyToo(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)
	if err := s.SetCupPrice(20); err != nil {
		t.Fatal(err)
	}
	sale, _, err := s.CreateSale(p.ID, 2, 3)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.VoidSale(sale.ID); err != nil {
		t.Fatal(err)
	}
	sum, err := s.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if sum.TodayRevenue != 0 {
		t.Fatalf("revenue after voiding = %d, want 0 (the cups' 60 must go too)", sum.TodayRevenue)
	}
	if got, _ := s.GetProduct(p.ID); got.Stock != 10 {
		t.Fatalf("stock after void = %d, want 10", got.Stock)
	}
}

func TestCupMoneyCountsInTodaysRevenue(t *testing.T) {
	s := newTestStore(t)
	p := mustCreateProduct(t, s, "Cola", "", 150, 10, 5)
	if err := s.SetCupPrice(20); err != nil {
		t.Fatal(err)
	}
	if _, _, err := s.CreateSale(p.ID, 2, 3); err != nil {
		t.Fatal(err)
	}
	sum, err := s.Summary()
	if err != nil {
		t.Fatal(err)
	}
	if sum.TodayRevenue != 360 {
		t.Fatalf("today's revenue = %d, want 360 (300 + 3 cups x 20)", sum.TodayRevenue)
	}
}
