//go:build windows

package winui

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/tailscale/walk"
	. "github.com/tailscale/walk/declarative"

	"shop/internal/client"
)

type sellPage struct {
	*shop
	search   *walk.LineEdit
	results  *walk.TableView
	found    *tableModel[client.Product]
	name     *walk.TextLabel
	qty      *walk.NumberEdit
	total    *walk.Label
	sellBtn  *walk.PushButton
	status   *walk.TextLabel
	recent   *walk.TableView
	recentTM *tableModel[client.Sale]
}

func newSellPage(s *shop) *sellPage {
	p := &sellPage{shop: s}
	p.found = &tableModel[client.Product]{cell: func(r client.Product, col int) string {
		switch col {
		case 0:
			return r.Name
		case 1:
			if r.SKU == "" {
				return "—"
			}
			return r.SKU
		case 2:
			return s.money(r.Price)
		case 3:
			return strconv.FormatInt(r.Stock, 10)
		}
		return ""
	}}
	p.recentTM = &tableModel[client.Sale]{cell: func(r client.Sale, col int) string {
		switch col {
		case 0:
			return clock(r.CreatedAt)
		case 1:
			return r.ProductName
		case 2:
			return strconv.FormatInt(r.Quantity, 10)
		case 3:
			return s.money(r.Total)
		case 4:
			if r.Voided {
				return "Voided"
			}
		}
		return ""
	}}
	return p
}

func (p *sellPage) tab() TabPage {
	return TabPage{
		Title:  "Sell",
		Layout: VBox{},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					Label{Text: "Search product:"},
					LineEdit{AssignTo: &p.search, CueBanner: "Name or SKU", MinSize: Size{Width: 280}, OnTextChanged: p.refreshProducts},
				},
			},
			TableView{
				AssignTo:         &p.results,
				Model:            p.found,
				AlternatingRowBG: true,
				Columns: []TableViewColumn{
					{Title: "Product", Width: 300},
					{Title: "SKU", Width: 110},
					{Title: "Price", Width: 140, Alignment: AlignFar},
					{Title: "In stock", Width: 80, Alignment: AlignFar},
				},
				OnCurrentIndexChanged: p.selectionChanged,
				OnItemActivated:       p.sell,
			},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					TextLabel{AssignTo: &p.name, Text: "Pick a product above.", MinSize: Size{Width: 300}},
					Label{Text: "Quantity:"},
					NumberEdit{
						AssignTo: &p.qty, Value: 1.0, MinValue: 1, MaxValue: 100000, Decimals: 0,
						SpinButtonsVisible: true, MinSize: Size{Width: 80}, OnValueChanged: p.updateTotal,
					},
					Label{Text: "Total:"},
					Label{AssignTo: &p.total, Text: "—", MinSize: Size{Width: 140}},
					PushButton{AssignTo: &p.sellBtn, Text: "Sell", Enabled: false, OnClicked: p.sell},
				},
			},
			TextLabel{AssignTo: &p.status, MinSize: Size{Width: 400}},
			Label{Text: "Recent sales"},
			TableView{
				AssignTo:         &p.recent,
				Model:            p.recentTM,
				AlternatingRowBG: true,
				MinSize:          Size{Height: 150},
				Columns: []TableViewColumn{
					{Title: "Time", Width: 70},
					{Title: "Product", Width: 300},
					{Title: "Qty", Width: 60, Alignment: AlignFar},
					{Title: "Total", Width: 140, Alignment: AlignFar},
					{Title: "", Width: 80},
				},
				StyleCell: func(style *walk.CellStyle) {
					if r, ok := p.recentTM.at(style.Row()); ok && r.Voided {
						style.TextColor = walk.RGB(130, 130, 130)
					}
				},
			},
		},
	}
}

func (p *sellPage) selected() (client.Product, bool) { return p.found.at(p.results.CurrentIndex()) }

// refresh reloads both lists.
func (p *sellPage) refresh() {
	p.refreshProducts()
	p.refreshRecent()
}

func (p *sellPage) refreshProducts() {
	if p.results == nil || p.search == nil || p.qty == nil {
		return // still being built
	}
	active := false
	list, err := p.c.Products(strings.TrimSpace(p.search.Text()), &active)
	if err != nil {
		p.showError(err)
		return
	}
	var keep int64 = -1
	if r, ok := p.selected(); ok {
		keep = r.ID
	}
	p.found.set(list)
	for i, r := range list {
		if r.ID == keep {
			_ = p.results.SetCurrentIndex(i)
			break
		}
	}
	p.selectionChanged()
}

func (p *sellPage) refreshRecent() {
	if p.recent == nil {
		return
	}
	list, err := p.c.Sales(10)
	if err != nil {
		p.showError(err)
		return
	}
	p.recentTM.set(list)
}

func (p *sellPage) selectionChanged() {
	if p.name == nil || p.qty == nil || p.sellBtn == nil || p.total == nil {
		return
	}
	r, ok := p.selected()
	if !ok {
		p.name.SetText("Pick a product above.")
		p.sellBtn.SetEnabled(false)
		p.total.SetText("—")
		return
	}
	p.name.SetText(fmt.Sprintf("%s — %s each", r.Name, p.money(r.Price)))
	if r.Stock > 0 {
		_ = p.qty.SetRange(1, float64(r.Stock))
		if p.qty.Value() > float64(r.Stock) || p.qty.Value() < 1 {
			_ = p.qty.SetValue(1)
		}
	}
	p.sellBtn.SetEnabled(r.Stock > 0)
	p.updateTotal()
}

func (p *sellPage) updateTotal() {
	if p.total == nil || p.qty == nil {
		return
	}
	r, ok := p.selected()
	if !ok {
		p.total.SetText("—")
		return
	}
	p.total.SetText(p.money(r.Price * int64(p.qty.Value())))
}

func (p *sellPage) sell() {
	r, ok := p.selected()
	if !ok || r.Stock <= 0 {
		return
	}
	qty := int64(p.qty.Value())
	sale, _, err := p.c.CreateSale(r.ID, qty)
	if err != nil {
		// Someone else may have sold the last units: show the real stock again.
		p.status.SetTextColor(walk.RGB(200, 40, 40))
		p.status.SetText(err.Error())
		p.refresh()
		return
	}
	p.status.SetTextColor(walk.RGB(30, 130, 60))
	p.status.SetText(fmt.Sprintf("Sold %d × %s — %s", sale.Quantity, sale.ProductName, p.money(sale.Total)))
	_ = p.qty.SetValue(1)
	p.refresh()
}
