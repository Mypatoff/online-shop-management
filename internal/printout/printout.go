// Package printout builds the printable pages (daily report, till receipts) as standalone HTML.
// The native UI has no printing of its own: it writes one of these pages to a temporary file
// and opens it in the browser, which prints it and shows the usual print dialog. The layouts are
// those of the web pages (web/static/sales.js, billiard.js).
package printout

import (
	"bytes"
	"fmt"
	"html/template"
	"time"

	"shop/internal/client"
)

// Money formats an amount for display, e.g. "150 000 so'm".
type Money func(int64) string

const css = `
body{font-family:system-ui,Segoe UI,Arial,sans-serif;color:#000;background:#fff;margin:1.5rem}
table{width:100%;border-collapse:collapse}
th,td{border:1px solid #000;padding:.4rem .6rem;text-align:left}
tr{break-inside:avoid}
thead{display:table-header-group}
.receipt{width:72mm;margin:0;font-size:12pt}
.receipt p{margin:.2rem 0}
`

const autoPrint = `<script>window.addEventListener("load",function(){window.print()});</script>`

var funcs = template.FuncMap{
	"clock": func(unix int64) string { return time.Unix(unix, 0).Local().Format("15:04") },
	"status": func(voided bool) string {
		if voided {
			return "Voided"
		}
		return "Completed"
	},
	"dash": func(s string) string {
		if s == "" {
			return "—"
		}
		return s
	},
	"minutes": func(m *int64) string {
		if m == nil {
			return "—"
		}
		return fmt.Sprint(*m)
	},
	"plural": func(n int, one, many string) string {
		if n == 1 {
			return one
		}
		return many
	},
}

var reportTmpl = template.Must(template.New("report").Funcs(funcs).Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>Daily sales report {{.Date}}</title>
<style>` + css + `</style>` + autoPrint + `</head><body>
<h1>{{.Shop}}</h1>
<p>Daily sales report — {{.Date}}</p>
<p>Printed at {{.Printed}}</p>
<ul>
<li>Revenue: {{.Revenue}}</li>
<li>Units sold: {{.Units}}</li>
<li>Sales: {{.SaleCount}}</li>
<li>Billiard revenue: {{.BilliardRevenue}}</li>
<li>Total revenue: shop + billiard = {{.TotalRevenue}}</li>
</ul>
{{if .VoidedSales}}<p>{{.VoidedSales}} voided {{plural .VoidedSales "sale" "sales"}} not counted.</p>{{end}}
{{if .VoidedBilliard}}<p>{{.VoidedBilliard}} voided billiard {{plural .VoidedBilliard "entry" "entries"}} not counted.</p>{{end}}
<h2>By product</h2>
<table><thead><tr><th>Product</th><th>Units</th><th>Revenue</th></tr></thead><tbody>
{{range .ByProduct}}<tr><td>{{.Name}}</td><td>{{.Units}}</td><td>{{call $.Fmt .Revenue}}</td></tr>
{{end}}</tbody></table>
<h2>Sales</h2>
<table><thead><tr><th>Time</th><th>Product</th><th>Qty</th><th>Unit price</th><th>Total</th><th>Status</th></tr></thead><tbody>
{{range .Sales}}<tr><td>{{clock .CreatedAt}}</td><td>{{.ProductName}}</td><td>{{.Quantity}}</td><td>{{call $.Fmt .UnitPrice}}</td><td>{{call $.Fmt .Total}}</td><td>{{status .Voided}}</td></tr>
{{end}}</tbody></table>
<h2>Billiard</h2>
<table><thead><tr><th>Time</th><th>Table</th><th>Minutes</th><th>Note</th><th>Amount</th><th>Status</th></tr></thead><tbody>
{{range .Billiard}}<tr><td>{{clock .CreatedAt}}</td><td>{{dash .Table}}</td><td>{{minutes .Minutes}}</td><td>{{dash .Note}}</td><td>{{call $.Fmt .Amount}}</td><td>{{status .Voided}}</td></tr>
{{end}}</tbody></table>
</body></html>
`))

var saleReceiptTmpl = template.Must(template.New("sale").Funcs(funcs).Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>Receipt</title>
<style>` + css + `@page{size:80mm auto;margin:4mm}</style>` + autoPrint + `</head><body><div class="receipt">
<p>{{.Shop}}</p>
<p>{{.When}}</p>
<p>{{.Sale.ProductName}}</p>
<p>Qty: {{.Sale.Quantity}}</p>
<p>Unit price: {{.UnitPrice}}</p>
<p>Total: {{.Total}}</p>
<p>Thank you</p>
</div></body></html>
`))

var billiardReceiptTmpl = template.Must(template.New("billiard").Funcs(funcs).Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8"><title>Receipt</title>
<style>` + css + `@page{size:80mm auto;margin:4mm}</style>` + autoPrint + `</head><body><div class="receipt">
<p>{{.Shop}}</p>
<p>{{.When}}</p>
<p>Billiard</p>
{{if .Entry.Table}}<p>Table: {{.Entry.Table}}</p>{{end}}
{{if .Entry.Minutes}}<p>Minutes: {{.Entry.Minutes}}</p>{{end}}
<p>Amount: {{.Amount}}</p>
<p>Thank you</p>
</div></body></html>
`))

func render(t *template.Template, data any) ([]byte, error) {
	var buf bytes.Buffer
	if err := t.Execute(&buf, data); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func stamp(t time.Time) string { return t.Format("2006-01-02 15:04:05") }

// DailyReport is the "Print daily report" page for one day.
func DailyReport(shop string, d client.Daily, fmtMoney Money, now time.Time) ([]byte, error) {
	voidedBilliard := 0
	for _, e := range d.Billiard.Entries {
		if e.Voided {
			voidedBilliard++
		}
	}
	return render(reportTmpl, map[string]any{
		"Shop": shop, "Date": d.Date, "Printed": stamp(now), "Fmt": fmtMoney,
		"Revenue":         fmtMoney(d.Totals.Revenue),
		"Units":           d.Totals.Units,
		"SaleCount":       d.Totals.SaleCount,
		"BilliardRevenue": fmtMoney(d.Billiard.Revenue),
		"TotalRevenue":    fmtMoney(d.Totals.TotalRevenue),
		"VoidedSales":     int(d.Totals.VoidedCount),
		"VoidedBilliard":  voidedBilliard,
		"ByProduct":       d.ByProduct,
		"Sales":           d.Sales,
		"Billiard":        d.Billiard.Entries,
	})
}

// SaleReceipt is the narrow till receipt for one sale.
func SaleReceipt(shop string, s client.Sale, fmtMoney Money) ([]byte, error) {
	return render(saleReceiptTmpl, map[string]any{
		"Shop": shop, "When": stamp(time.Unix(s.CreatedAt, 0).Local()), "Sale": s,
		"UnitPrice": fmtMoney(s.UnitPrice), "Total": fmtMoney(s.Total),
	})
}

// BilliardReceipt is the narrow till receipt for one billiard payment.
func BilliardReceipt(shop string, e client.BilliardEntry, fmtMoney Money) ([]byte, error) {
	return render(billiardReceiptTmpl, map[string]any{
		"Shop": shop, "When": stamp(time.Unix(e.CreatedAt, 0).Local()), "Entry": e,
		"Amount": fmtMoney(e.Amount),
	})
}
