package printout

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"shop/internal/client"
)

func money(n int64) string { return fmt.Sprintf("%d so'm", n) }

func i64(n int64) *int64 { return &n }

func sampleDay() client.Daily {
	at := time.Date(2026, 11, 6, 14, 5, 0, 0, time.Local).Unix()
	return client.Daily{
		Date:   "2026-11-06",
		Totals: client.DailyTotals{Revenue: 9000, Units: 3, SaleCount: 2, VoidedCount: 1, TotalRevenue: 39000},
		ByProduct: []client.DailyProduct{
			{Name: "Cola <b>", Units: 3, Revenue: 9000},
		},
		Sales: []client.Sale{
			{ProductName: "Cola <b>", Quantity: 3, UnitPrice: 3000, Total: 9000, CreatedAt: at},
			{ProductName: "Tea", Quantity: 1, UnitPrice: 2000, Total: 2000, CreatedAt: at, Voided: true},
		},
		Billiard: client.DailyBilliard{
			Revenue: 30000, Count: 1,
			Entries: []client.BilliardEntry{
				{Amount: 30000, Table: "T1", Minutes: i64(60), CreatedAt: at},
				{Amount: 10000, CreatedAt: at, Voided: true},
			},
		},
	}
}

func TestDailyReport(t *testing.T) {
	out, err := DailyReport("Shop & Co", sampleDay(), money, time.Date(2026, 11, 6, 20, 0, 0, 0, time.Local))
	if err != nil {
		t.Fatal(err)
	}
	html := string(out)
	for _, want := range []string{
		"<h1>Shop &amp; Co</h1>",
		"Daily sales report — 2026-11-06",
		"Printed at 2026-11-06 20:00:00",
		"Revenue: 9000 so&#39;m",
		"Units sold: 3",
		"Billiard revenue: 30000 so&#39;m",
		"Total revenue: shop + billiard = 39000 so&#39;m",
		"1 voided sale not counted.",
		"1 voided billiard entry not counted.",
		"<td>Cola &lt;b&gt;</td>", // markup in a product name is escaped
		"<td>14:05</td>",
		"<td>Voided</td>",
		"<td>Completed</td>",
		"<td>T1</td><td>60</td>",
		"<td>—</td>", // missing table, minutes and note
		"window.print()",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("report missing %q", want)
		}
	}
	if strings.Contains(html, "<b>") {
		t.Error("product name was not escaped")
	}
}

func TestDailyReportPlurals(t *testing.T) {
	d := sampleDay()
	d.Totals.VoidedCount = 2
	d.Billiard.Entries = append(d.Billiard.Entries, client.BilliardEntry{Voided: true})
	out, _ := DailyReport("S", d, money, time.Now())
	for _, want := range []string{"2 voided sales not counted.", "2 voided billiard entries not counted."} {
		if !strings.Contains(string(out), want) {
			t.Errorf("missing %q", want)
		}
	}
	d.Totals.VoidedCount = 0
	d.Billiard.Entries = nil
	out, _ = DailyReport("S", d, money, time.Now())
	if strings.Contains(string(out), "voided") {
		t.Error("no voided line expected when nothing is voided")
	}
}

func TestReceipts(t *testing.T) {
	at := time.Date(2026, 11, 6, 9, 30, 15, 0, time.Local).Unix()
	out, err := SaleReceipt("Shop", client.Sale{ProductName: "Tea", Quantity: 2, UnitPrice: 2000, Total: 4000, CreatedAt: at}, money)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<p>Shop</p>", "2026-11-06 09:30:15", "<p>Tea</p>", "Qty: 2", "Unit price: 2000", "Total: 4000", "Thank you", "@page{size:80mm auto;margin:4mm}", "window.print()"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("sale receipt missing %q", want)
		}
	}

	out, err = BilliardReceipt("Shop", client.BilliardEntry{Amount: 30000, Table: "T1", Minutes: i64(45), CreatedAt: at}, money)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"<p>Billiard</p>", "Table: T1", "Minutes: 45", "Amount: 30000", "Thank you"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("billiard receipt missing %q", want)
		}
	}
	out, _ = BilliardReceipt("Shop", client.BilliardEntry{Amount: 1, CreatedAt: at}, money)
	if strings.Contains(string(out), "Table:") || strings.Contains(string(out), "Minutes:") {
		t.Error("table and minutes lines must be left out when not recorded")
	}
}
