// Package client is a typed, in-process client for ShopKeeper's JSON API. The native Windows UI
// uses it so that validation, error messages and business rules stay in one place
// (internal/api): a call goes straight into the API's http.Handler, with no network socket.
package client

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
)

// Client calls an API handler directly.
type Client struct {
	h    http.Handler
	host string
}

// New returns a client for h. host must be one the handler accepts (see api.Routes), such as
// "localhost:8080".
func New(h http.Handler, host string) *Client { return &Client{h: h, host: host} }

// Error is a failed call; Message is the API's own text, ready to show to the user.
type Error struct {
	Status  int
	Message string
}

func (e *Error) Error() string { return e.Message }

// do runs one request. body (if not nil) is sent as JSON; a 2xx answer is decoded into out
// (if not nil); anything else becomes an *Error.
func (c *Client) do(method, path string, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rd)
	req.Host = c.host
	req.Header.Set("Accept", "application/json")
	if method == http.MethodPost || method == http.MethodPut {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)

	if rec.Code < 200 || rec.Code > 299 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		if e.Error == "" {
			e.Error = fmt.Sprintf("Request failed (%d).", rec.Code)
		}
		return &Error{Status: rec.Code, Message: e.Error}
	}
	if out == nil {
		return nil
	}
	if err := json.Unmarshal(rec.Body.Bytes(), out); err != nil {
		return fmt.Errorf("unexpected answer from %s: %w", path, err)
	}
	return nil
}

// IsStatus reports whether err is an API error with the given HTTP status (e.g. 409).
func IsStatus(err error, status int) bool {
	var e *Error
	return errors.As(err, &e) && e.Status == status
}

// ---- types (mirrors of the API's JSON) ----

type Config struct {
	ShopName string `json:"shop_name"`
	Currency string `json:"currency"`
	Decimals int    `json:"decimals"`
}

type Product struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	SKU               string `json:"sku"`
	Price             int64  `json:"price"`
	Stock             int64  `json:"stock"`
	LowStockThreshold int64  `json:"low_stock_threshold"`
	Archived          bool   `json:"archived"`
	CreatedAt         int64  `json:"created_at"`
	UpdatedAt         int64  `json:"updated_at"`
}

type Sale struct {
	ID          int64  `json:"id"`
	ProductID   int64  `json:"product_id"`
	ProductName string `json:"product_name"`
	Quantity    int64  `json:"quantity"`
	UnitPrice   int64  `json:"unit_price"`
	Total       int64  `json:"total"`
	CreatedAt   int64  `json:"created_at"`
	Voided      bool   `json:"voided"`
}

type BilliardEntry struct {
	ID        int64  `json:"id"`
	Amount    int64  `json:"amount"`
	Table     string `json:"table"`
	Minutes   *int64 `json:"minutes"`
	Note      string `json:"note"`
	CreatedAt int64  `json:"created_at"`
	Voided    bool   `json:"voided"`
}

type LowStockItem struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	SKU               string `json:"sku"`
	Stock             int64  `json:"stock"`
	LowStockThreshold int64  `json:"low_stock_threshold"`
}

type Summary struct {
	ActiveProducts       int64          `json:"active_products"`
	UnitsInStock         int64          `json:"units_in_stock"`
	StockValue           int64          `json:"stock_value"`
	TodayRevenue         int64          `json:"today_revenue"`
	TodaySaleCount       int64          `json:"today_sale_count"`
	BilliardRevenueToday int64          `json:"billiard_revenue_today"`
	BilliardCountToday   int64          `json:"billiard_count_today"`
	TotalRevenueToday    int64          `json:"total_revenue_today"`
	LowStock             []LowStockItem `json:"low_stock"`
}

type DailyTotals struct {
	Revenue      int64 `json:"revenue"`
	Units        int64 `json:"units"`
	SaleCount    int64 `json:"sale_count"`
	VoidedCount  int64 `json:"voided_count"`
	TotalRevenue int64 `json:"total_revenue"`
}

type DailyProduct struct {
	Name    string `json:"name"`
	Units   int64  `json:"units"`
	Revenue int64  `json:"revenue"`
}

type DailyBilliard struct {
	Entries []BilliardEntry `json:"entries"`
	Revenue int64           `json:"revenue"`
	Count   int64           `json:"count"`
}

// Daily is one local calendar day's sales (oldest first, voided included).
type Daily struct {
	Date      string         `json:"date"`
	Sales     []Sale         `json:"sales"`
	Totals    DailyTotals    `json:"totals"`
	ByProduct []DailyProduct `json:"by_product"`
	Billiard  DailyBilliard  `json:"billiard"`
}

type BilliardDay struct {
	Date    string          `json:"date"`
	Entries []BilliardEntry `json:"entries"`
	Totals  struct {
		Revenue int64 `json:"revenue"`
		Count   int64 `json:"count"`
	} `json:"totals"`
	Tables []string `json:"tables"`
}

type StockMovement struct {
	ID          int64  `json:"id"`
	ProductID   int64  `json:"product_id"`
	ProductName string `json:"product_name"`
	Type        string `json:"type"`
	Delta       int64  `json:"delta"`
	StockAfter  int64  `json:"stock_after"`
	Reason      string `json:"reason"`
	Note        string `json:"note"`
	SaleID      *int64 `json:"sale_id"`
	CreatedAt   int64  `json:"created_at"`
}

type ChartEntry struct {
	Date            string `json:"date"`
	Revenue         int64  `json:"revenue"`
	BilliardRevenue int64  `json:"billiard_revenue"`
	Units           int64  `json:"units"`
	SaleCount       int64  `json:"sale_count"`
}

// ---- calls ----

func (c *Client) Config() (Config, error) {
	var out Config
	return out, c.do("GET", "/api/config", nil, &out)
}

func (c *Client) Summary() (Summary, error) {
	var out Summary
	return out, c.do("GET", "/api/summary", nil, &out)
}

// Products lists products matching q (name or SKU). archived nil means both.
func (c *Client) Products(q string, archived *bool) ([]Product, error) {
	v := url.Values{}
	if q != "" {
		v.Set("q", q)
	}
	if archived != nil {
		if *archived {
			v.Set("archived", "1")
		} else {
			v.Set("archived", "0")
		}
	}
	var out []Product
	return out, c.do("GET", "/api/products?"+v.Encode(), nil, &out)
}

// ProductInput is the editable part of a product. LowStockThreshold nil means the default.
type ProductInput struct {
	Name              string `json:"name"`
	SKU               string `json:"sku"`
	Price             int64  `json:"price"`
	Stock             int64  `json:"stock"` // only used when creating
	LowStockThreshold *int64 `json:"low_stock_threshold"`
}

func (c *Client) CreateProduct(in ProductInput) (Product, error) {
	var out Product
	return out, c.do("POST", "/api/products", in, &out)
}

func (c *Client) UpdateProduct(id int64, in ProductInput) (Product, error) {
	// The update route takes a plain threshold and has no stock field.
	var threshold int64 = 5
	if in.LowStockThreshold != nil {
		threshold = *in.LowStockThreshold
	}
	body := struct {
		Name              string `json:"name"`
		SKU               string `json:"sku"`
		Price             int64  `json:"price"`
		LowStockThreshold int64  `json:"low_stock_threshold"`
	}{in.Name, in.SKU, in.Price, threshold}
	var out Product
	return out, c.do("PUT", "/api/products/"+strconv.FormatInt(id, 10), body, &out)
}

// AdjustStock changes a product's stock by delta (not zero) and returns the new stock.
func (c *Client) AdjustStock(id, delta int64, reason, note string) (int64, error) {
	body := map[string]any{"delta": delta, "reason": reason, "note": note}
	var out struct {
		Stock int64 `json:"stock"`
	}
	err := c.do("POST", "/api/products/"+strconv.FormatInt(id, 10)+"/adjust", body, &out)
	return out.Stock, err
}

func (c *Client) SetArchived(id int64, archived bool) error {
	action := "unarchive"
	if archived {
		action = "archive"
	}
	return c.do("POST", "/api/products/"+strconv.FormatInt(id, 10)+"/"+action, map[string]any{}, nil)
}

// CreateSale sells quantity units and returns the sale and the product's new stock.
func (c *Client) CreateSale(productID, quantity int64) (Sale, int64, error) {
	var out struct {
		Sale  Sale  `json:"sale"`
		Stock int64 `json:"stock"`
	}
	err := c.do("POST", "/api/sales", map[string]int64{"product_id": productID, "quantity": quantity}, &out)
	return out.Sale, out.Stock, err
}

func (c *Client) VoidSale(id int64) error {
	return c.do("POST", "/api/sales/"+strconv.FormatInt(id, 10)+"/void", map[string]any{}, nil)
}

// Sales lists the most recent sales, newest first, voided included.
func (c *Client) Sales(limit int) ([]Sale, error) {
	var out []Sale
	return out, c.do("GET", "/api/sales?limit="+strconv.Itoa(limit), nil, &out)
}

// Daily reports the day date ("YYYY-MM-DD"; "" is today).
func (c *Client) Daily(date string) (Daily, error) {
	var out Daily
	return out, c.do("GET", "/api/sales/daily"+dateQuery(date), nil, &out)
}

// Chart returns the last 7 or 30 days, oldest first.
func (c *Client) Chart(days int) ([]ChartEntry, error) {
	var out []ChartEntry
	return out, c.do("GET", "/api/sales/chart?days="+strconv.Itoa(days), nil, &out)
}

// Billiard lists the day's billiard entries ("" is today).
func (c *Client) Billiard(date string) (BilliardDay, error) {
	var out BilliardDay
	return out, c.do("GET", "/api/billiard"+dateQuery(date), nil, &out)
}

// BilliardInput is a payment for table time. Minutes nil means not recorded.
type BilliardInput struct {
	Amount  int64  `json:"amount"`
	Table   string `json:"table"`
	Minutes *int64 `json:"minutes"`
	Note    string `json:"note"`
}

func (c *Client) CreateBilliard(in BilliardInput) (BilliardEntry, error) {
	var out BilliardEntry
	return out, c.do("POST", "/api/billiard", in, &out)
}

func (c *Client) VoidBilliard(id int64) error {
	return c.do("POST", "/api/billiard/"+strconv.FormatInt(id, 10)+"/void", map[string]any{}, nil)
}

// StockLog lists stock movements newest first. next is set when more may follow: pass it as
// beforeID to get the next page.
func (c *Client) StockLog(productID *int64, limit int, beforeID *int64) (movements []StockMovement, next *int64, err error) {
	v := url.Values{}
	if productID != nil {
		v.Set("product_id", strconv.FormatInt(*productID, 10))
	}
	if limit > 0 {
		v.Set("limit", strconv.Itoa(limit))
	}
	if beforeID != nil {
		v.Set("before_id", strconv.FormatInt(*beforeID, 10))
	}
	var out struct {
		Movements    []StockMovement `json:"movements"`
		NextBeforeID *int64          `json:"next_before_id"`
	}
	err = c.do("GET", "/api/stock-log?"+v.Encode(), nil, &out)
	return out.Movements, out.NextBeforeID, err
}

// BackupTo writes a consistent snapshot of the database to path. The file appears only when it
// is complete.
func (c *Client) BackupTo(path string) error {
	req := httptest.NewRequest("GET", "/api/backup", nil)
	req.Host = c.host
	rec := httptest.NewRecorder()
	c.h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(rec.Body.Bytes(), &e)
		if e.Error == "" {
			e.Error = "could not create backup"
		}
		return &Error{Status: rec.Code, Message: e.Error}
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".shop-backup-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name()) //nolint:errcheck // gone after the rename; best effort otherwise
	if _, err := tmp.Write(rec.Body.Bytes()); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func dateQuery(date string) string {
	if date == "" {
		return ""
	}
	return "?date=" + url.QueryEscape(date)
}
