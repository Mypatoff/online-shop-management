//go:build windows

package winui

import (
	"strconv"
	"strings"

	"github.com/tailscale/walk"
	. "github.com/tailscale/walk/declarative"

	"shop/internal/client"
	"shop/internal/money"
)

var adjustReasons = []formOption{
	{"restock", "Restock"},
	{"recount", "Recount"},
	{"damaged", "Damaged"},
	{"other", "Other"},
}

type productsPage struct {
	*shop
	search       *walk.LineEdit
	showArchived *walk.CheckBox
	table        *walk.TableView
	model        *tableModel[client.Product]
	archiveBtn   *walk.PushButton
}

func newProductsPage(s *shop) *productsPage {
	p := &productsPage{shop: s}
	p.model = &tableModel[client.Product]{cell: func(r client.Product, col int) string {
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
		case 4:
			if r.Archived {
				return "Archived"
			}
			return ""
		}
		return ""
	}}
	return p
}

func (p *productsPage) tab() TabPage {
	return TabPage{
		Title:  "Products",
		Layout: VBox{},
		Children: []Widget{
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					Label{Text: "Search:"},
					LineEdit{AssignTo: &p.search, CueBanner: "Name or SKU", MinSize: Size{Width: 240}, OnTextChanged: p.refresh},
					CheckBox{AssignTo: &p.showArchived, Text: "Show archived", OnCheckedChanged: p.refresh},
					HSpacer{},
					PushButton{Text: "Add product", OnClicked: p.add},
				},
			},
			TableView{
				AssignTo:         &p.table,
				Model:            p.model,
				AlternatingRowBG: true,
				ColumnsOrderable: false,
				Columns: []TableViewColumn{
					{Title: "Name", Width: 300},
					{Title: "SKU", Width: 110},
					{Title: "Price", Width: 140, Alignment: AlignFar},
					{Title: "Stock", Width: 80, Alignment: AlignFar},
					{Title: "", Width: 90},
				},
				StyleCell:             p.styleCell,
				OnItemActivated:       p.edit,
				OnCurrentIndexChanged: p.selectionChanged,
			},
			Composite{
				Layout: HBox{MarginsZero: true},
				Children: []Widget{
					PushButton{Text: "Edit", OnClicked: p.edit},
					PushButton{Text: "Adjust stock", OnClicked: p.adjust},
					PushButton{AssignTo: &p.archiveBtn, Text: "Archive", OnClicked: p.toggleArchive},
					HSpacer{},
				},
			},
		},
	}
}

func (p *productsPage) styleCell(style *walk.CellStyle) {
	r, ok := p.model.at(style.Row())
	if !ok {
		return
	}
	if r.Archived {
		style.TextColor = walk.RGB(130, 130, 130)
		return
	}
	if style.Col() == 3 {
		switch {
		case r.Stock <= 0:
			style.TextColor = walk.RGB(200, 40, 40)
		case r.Stock <= r.LowStockThreshold:
			style.TextColor = walk.RGB(190, 120, 0)
		}
	}
}

func (p *productsPage) selected() (client.Product, bool) {
	return p.model.at(p.table.CurrentIndex())
}

func (p *productsPage) selectionChanged() {
	if r, ok := p.selected(); ok && r.Archived {
		p.archiveBtn.SetText("Unarchive")
	} else {
		p.archiveBtn.SetText("Archive")
	}
}

// refresh reloads the table, keeping the selected product selected.
func (p *productsPage) refresh() {
	if p.table == nil || p.search == nil || p.showArchived == nil {
		return // still being built
	}
	var archived *bool
	if !p.showArchived.Checked() {
		no := false
		archived = &no // "show archived" off: active products only
	}
	list, err := p.c.Products(strings.TrimSpace(p.search.Text()), archived)
	if err != nil {
		p.showError(err)
		return
	}
	var keep int64 = -1
	if r, ok := p.selected(); ok {
		keep = r.ID
	}
	p.model.set(list)
	for i, r := range list {
		if r.ID == keep {
			_ = p.table.SetCurrentIndex(i)
			break
		}
	}
	p.selectionChanged()
}

func (p *productsPage) add() {
	fields := []formField{
		{name: "name", label: "Name"},
		{name: "sku", label: "SKU (optional)"},
		{name: "price", label: "Price"},
		{name: "stock", label: "Starting stock", value: "0"},
		{name: "low_stock_threshold", label: "Low stock threshold", value: "5"},
	}
	if p.formDialog("Add product", "Add", fields, func(v map[string]string) error {
		in, err := p.validateProduct(v, true)
		if err != nil {
			return err
		}
		_, err = p.c.CreateProduct(in)
		return err
	}) {
		p.refresh()
	}
}

func (p *productsPage) edit() {
	r, ok := p.selected()
	if !ok {
		return
	}
	fields := []formField{
		{name: "name", label: "Name", value: r.Name},
		{name: "sku", label: "SKU (optional)", value: r.SKU},
		{name: "price", label: "Price", value: money.FormatInput(r.Price, p.cfg.Decimals)},
		{name: "low_stock_threshold", label: "Low stock threshold", value: strconv.FormatInt(r.LowStockThreshold, 10)},
	}
	if p.formDialog("Edit product", "Save", fields, func(v map[string]string) error {
		in, err := p.validateProduct(v, false)
		if err != nil {
			return err
		}
		_, err = p.c.UpdateProduct(r.ID, in)
		return err
	}) {
		p.refresh()
	}
}

func (p *productsPage) adjust() {
	r, ok := p.selected()
	if !ok {
		return
	}
	if p.adjustStock(r) {
		p.refresh()
	}
}

// adjustStock asks how much to add or remove and applies it. True if stock changed.
func (s *shop) adjustStock(r client.Product) bool {
	fields := []formField{
		{name: "delta", label: "Change stock by (e.g. 10 or -2)"},
		{name: "reason", label: "Reason", value: "restock", options: adjustReasons},
		{name: "note", label: "Note (optional)"},
	}
	return s.formDialog("Adjust stock — "+r.Name, "Apply", fields, func(v map[string]string) error {
		delta, err := money.ParseWhole(v["delta"], true, "Enter a whole number like 10 or -2.")
		if err != nil {
			return fieldErr("delta", err.Error())
		}
		if delta == 0 {
			return fieldErr("delta", "Enter a non-zero number.")
		}
		note := strings.TrimSpace(v["note"])
		if len([]rune(note)) > 200 {
			return fieldErr("note", "Note must be at most 200 characters.")
		}
		_, err = s.c.AdjustStock(r.ID, delta, v["reason"], note)
		return err
	})
}

func (p *productsPage) toggleArchive() {
	r, ok := p.selected()
	if !ok {
		return
	}
	if err := p.c.SetArchived(r.ID, !r.Archived); err != nil {
		p.showError(err)
		return
	}
	p.refresh()
}

// validateProduct applies the same limits as the server so mistakes show next to the field
// before anything is sent (the server checks again).
func (s *shop) validateProduct(v map[string]string, withStock bool) (client.ProductInput, error) {
	name := strings.TrimSpace(v["name"])
	if n := len([]rune(name)); n < 1 || n > 100 {
		return client.ProductInput{}, fieldErr("name", "Name must be 1-100 characters.")
	}
	sku := strings.TrimSpace(v["sku"])
	if len([]rune(sku)) > 40 {
		return client.ProductInput{}, fieldErr("sku", "SKU must be at most 40 characters.")
	}
	price, err := s.parseMoney(v["price"])
	if err != nil {
		return client.ProductInput{}, fieldErr("price", err.Error())
	}
	if price < 0 || price > 100_000_000_000 {
		return client.ProductInput{}, fieldErr("price", "Price must be between 0 and 100,000,000,000.")
	}
	threshold, err := money.ParseWhole(v["low_stock_threshold"], false, "")
	if err != nil {
		return client.ProductInput{}, fieldErr("low_stock_threshold", err.Error())
	}
	if threshold > 1_000_000 {
		return client.ProductInput{}, fieldErr("low_stock_threshold", "Threshold must be between 0 and 1,000,000.")
	}
	in := client.ProductInput{Name: name, SKU: sku, Price: price, LowStockThreshold: &threshold}
	if withStock {
		stock, err := money.ParseWhole(v["stock"], false, "")
		if err != nil {
			return client.ProductInput{}, fieldErr("stock", err.Error())
		}
		if stock > 1_000_000 {
			return client.ProductInput{}, fieldErr("stock", "Stock must be between 0 and 1,000,000.")
		}
		in.Stock = stock
	}
	return in, nil
}
