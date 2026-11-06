package client

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"shop/internal/api"
	"shop/internal/db"
	"shop/internal/store"
)

const testHost = "localhost:8080"

func newClient(t *testing.T) *Client {
	t.Helper()
	conn, err := db.Open(":memory:", "")
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	t.Cleanup(func() { conn.Close() })
	a := api.New(store.New(conn), "Test Shop", "so'm", 0)
	return New(a.Routes(8080), testHost)
}

func i64(n int64) *int64 { return &n }

func mustProduct(t *testing.T, c *Client, name string, price, stock int64) Product {
	t.Helper()
	p, err := c.CreateProduct(ProductInput{Name: name, Price: price, Stock: stock})
	if err != nil {
		t.Fatalf("create product: %v", err)
	}
	return p
}

func TestConfig(t *testing.T) {
	c := newClient(t)
	cfg, err := c.Config()
	if err != nil || cfg.ShopName != "Test Shop" || cfg.Currency != "so'm" || cfg.Decimals != 0 {
		t.Fatalf("config = %+v, %v", cfg, err)
	}
}

func TestWrongHostIsRefused(t *testing.T) {
	c := newClient(t)
	c.host = "evil.example.com:8080"
	if _, err := c.Config(); !IsStatus(err, 403) {
		t.Fatalf("err = %v, want a 403", err)
	}
}

func TestProductLifecycle(t *testing.T) {
	c := newClient(t)
	p, err := c.CreateProduct(ProductInput{Name: "  Cola ", SKU: "C1", Price: 5000, Stock: 10, LowStockThreshold: i64(3)})
	if err != nil {
		t.Fatal(err)
	}
	if p.Name != "Cola" || p.Stock != 10 || p.LowStockThreshold != 3 {
		t.Fatalf("product = %+v", p)
	}

	if _, err := c.CreateProduct(ProductInput{Name: "Other", SKU: "C1", Price: 1}); !IsStatus(err, 409) || err.Error() != "sku already in use" {
		t.Fatalf("duplicate sku: %v", err)
	}
	if _, err := c.CreateProduct(ProductInput{Name: "", Price: 1}); !IsStatus(err, 400) {
		t.Fatalf("empty name: %v", err)
	}

	upd, err := c.UpdateProduct(p.ID, ProductInput{Name: "Cola 0.5", SKU: "C1", Price: 6000})
	if err != nil || upd.Name != "Cola 0.5" || upd.Price != 6000 || upd.LowStockThreshold != 5 {
		t.Fatalf("update = %+v, %v", upd, err)
	}

	stock, err := c.AdjustStock(p.ID, -4, "damaged", "dropped")
	if err != nil || stock != 6 {
		t.Fatalf("adjust = %d, %v", stock, err)
	}
	if _, err := c.AdjustStock(p.ID, -100, "damaged", ""); !IsStatus(err, 409) {
		t.Fatalf("negative stock: %v", err)
	}
	if _, err := c.AdjustStock(p.ID, 0, "damaged", ""); !IsStatus(err, 400) {
		t.Fatalf("zero delta: %v", err)
	}

	if err := c.SetArchived(p.ID, true); err != nil {
		t.Fatal(err)
	}
	active := false
	if list, _ := c.Products("", &active); len(list) != 0 {
		t.Fatalf("archived product listed as active: %+v", list)
	}
	archived := true
	if list, _ := c.Products("cola", &archived); len(list) != 1 {
		t.Fatalf("archived search = %+v", list)
	}
	if err := c.SetArchived(p.ID, false); err != nil {
		t.Fatal(err)
	}
	if list, err := c.Products("", nil); err != nil || len(list) != 1 {
		t.Fatalf("all products = %+v, %v", list, err)
	}
}

func TestSellVoidAndDaily(t *testing.T) {
	c := newClient(t)
	p := mustProduct(t, c, "Tea", 2000, 5)

	sale, stock, err := c.CreateSale(p.ID, 2)
	if err != nil || stock != 3 || sale.Total != 4000 || sale.ProductName != "Tea" {
		t.Fatalf("sale = %+v stock %d, %v", sale, stock, err)
	}
	if _, _, err := c.CreateSale(p.ID, 99); !IsStatus(err, 409) || err.Error() != "not enough stock" {
		t.Fatalf("oversell: %v", err)
	}
	if _, _, err := c.CreateSale(p.ID, 0); !IsStatus(err, 400) {
		t.Fatalf("zero quantity: %v", err)
	}

	recent, err := c.Sales(10)
	if err != nil || len(recent) != 1 || recent[0].ID != sale.ID {
		t.Fatalf("recent = %+v, %v", recent, err)
	}

	day, err := c.Daily("")
	if err != nil || day.Totals.Revenue != 4000 || day.Totals.Units != 2 || len(day.ByProduct) != 1 {
		t.Fatalf("daily = %+v, %v", day, err)
	}
	if _, err := c.Daily("not-a-date"); !IsStatus(err, 400) {
		t.Fatalf("bad date: %v", err)
	}

	if err := c.VoidSale(sale.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.VoidSale(sale.ID); !IsStatus(err, 409) {
		t.Fatalf("second void: %v", err)
	}
	if err := c.VoidSale(9999); !IsStatus(err, 404) {
		t.Fatalf("unknown sale: %v", err)
	}
	day, _ = c.Daily(time.Now().Format("2006-01-02"))
	if day.Totals.Revenue != 0 || day.Totals.VoidedCount != 1 {
		t.Fatalf("after void: %+v", day.Totals)
	}
	if list, _ := c.Products("", nil); list[0].Stock != 5 {
		t.Fatalf("stock after void = %d, want 5", list[0].Stock)
	}
}

func TestBilliard(t *testing.T) {
	c := newClient(t)
	e, err := c.CreateBilliard(BilliardInput{Amount: 30000, Table: "T1", Minutes: i64(60), Note: "lunch"})
	if err != nil || e.Amount != 30000 || e.Table != "T1" || e.Minutes == nil || *e.Minutes != 60 {
		t.Fatalf("entry = %+v, %v", e, err)
	}
	if _, err := c.CreateBilliard(BilliardInput{Amount: 0}); !IsStatus(err, 400) {
		t.Fatalf("zero amount: %v", err)
	}
	day, err := c.Billiard("")
	if err != nil || day.Totals.Revenue != 30000 || day.Totals.Count != 1 || len(day.Tables) != 1 || day.Tables[0] != "T1" {
		t.Fatalf("day = %+v, %v", day, err)
	}
	if err := c.VoidBilliard(e.ID); err != nil {
		t.Fatal(err)
	}
	if err := c.VoidBilliard(e.ID); !IsStatus(err, 409) {
		t.Fatalf("second void: %v", err)
	}
	if err := c.VoidBilliard(12345); !IsStatus(err, 404) {
		t.Fatalf("unknown entry: %v", err)
	}
}

func TestSummaryChartAndStockLog(t *testing.T) {
	c := newClient(t)
	p := mustProduct(t, c, "Chips", 1000, 2)
	if _, _, err := c.CreateSale(p.ID, 1); err != nil {
		t.Fatal(err)
	}
	if _, err := c.CreateBilliard(BilliardInput{Amount: 500}); err != nil {
		t.Fatal(err)
	}

	sum, err := c.Summary()
	if err != nil || sum.TodayRevenue != 1000 || sum.BilliardRevenueToday != 500 || sum.TotalRevenueToday != 1500 {
		t.Fatalf("summary = %+v, %v", sum, err)
	}

	chart, err := c.Chart(7)
	if err != nil || len(chart) != 7 {
		t.Fatalf("chart = %+v, %v", chart, err)
	}
	if last := chart[len(chart)-1]; last.Revenue != 1000 || last.BilliardRevenue != 500 || last.Units != 1 {
		t.Fatalf("today's bar = %+v", last)
	}
	if _, err := c.Chart(5); !IsStatus(err, 400) {
		t.Fatalf("chart(5): %v", err)
	}

	log, next, err := c.StockLog(&p.ID, 100, nil)
	if err != nil || len(log) != 2 || next != nil { // initial balance + sale
		t.Fatalf("log = %+v next %v, %v", log, next, err)
	}
	page, next, err := c.StockLog(&p.ID, 1, nil)
	if err != nil || len(page) != 1 || next == nil {
		t.Fatalf("page 1 = %+v next %v, %v", page, next, err)
	}
	page2, _, err := c.StockLog(&p.ID, 1, next)
	if err != nil || len(page2) != 1 || page2[0].ID == page[0].ID {
		t.Fatalf("page 2 = %+v, %v", page2, err)
	}
}

func TestBackupTo(t *testing.T) {
	c := newClient(t)
	mustProduct(t, c, "Gum", 100, 1)
	path := filepath.Join(t.TempDir(), "out.db")
	if err := c.BackupTo(path); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(b), "SQLite format 3") {
		t.Fatalf("backup file: %d bytes, %v", len(b), err)
	}
	entries, _ := os.ReadDir(filepath.Dir(path))
	if len(entries) != 1 {
		t.Fatalf("temp file left behind: %v", entries)
	}
	if err := c.BackupTo(filepath.Join(t.TempDir(), "missing", "x.db")); err == nil {
		t.Fatal("want an error when the folder does not exist")
	}
}
