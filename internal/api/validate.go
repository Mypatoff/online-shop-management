package api

import (
	"fmt"
	"strings"
)

func validateName(name string) (string, error) {
	name = strings.TrimSpace(name)
	if len(name) < 1 || len(name) > 100 {
		return "", fmt.Errorf("name must be 1-100 characters")
	}
	return name, nil
}

func validateSKU(sku string) (string, error) {
	sku = strings.TrimSpace(sku)
	if len(sku) > 40 {
		return "", fmt.Errorf("sku must be at most 40 characters")
	}
	return sku, nil
}

func validateRange(field string, v, min, max int64) error {
	if v < min || v > max {
		return fmt.Errorf("%s must be between %d and %d", field, min, max)
	}
	return nil
}

const (
	maxPrice           = 1_000_000_000
	maxStock           = 1_000_000
	maxThreshold       = 1_000_000
	maxQuantity        = 100_000
	maxBilliardAmount  = 1_000_000_000
	maxBilliardMinutes = 1440
	maxTableNameLength = 30
)

func validatePrice(v int64) error { return validateRange("price", v, 0, maxPrice) }
func validateStock(v int64) error { return validateRange("stock", v, 0, maxStock) }
func validateThreshold(v int64) error {
	return validateRange("low_stock_threshold", v, 0, maxThreshold)
}
func validateQuantity(v int64) error { return validateRange("quantity", v, 1, maxQuantity) }

func validateBilliardAmount(v int64) error {
	return validateRange("amount", v, 1, maxBilliardAmount)
}

// validateTableName trims and bounds table, returning "" (stored as
// NULL) when empty.
func validateTableName(table string) (string, error) {
	table = strings.TrimSpace(table)
	if len(table) > maxTableNameLength {
		return "", fmt.Errorf("table must be at most %d characters", maxTableNameLength)
	}
	return table, nil
}

// validateMinutes allows minutes to be omitted (nil).
func validateMinutes(minutes *int64) error {
	if minutes == nil {
		return nil
	}
	return validateRange("minutes", *minutes, 0, maxBilliardMinutes)
}

var validAdjustReasons = map[string]bool{
	"restock": true,
	"recount": true,
	"damaged": true,
	"other":   true,
}

func validateAdjustReason(reason string) (string, error) {
	reason = strings.TrimSpace(reason)
	if !validAdjustReasons[reason] {
		return "", fmt.Errorf("reason must be one of restock, recount, damaged, other")
	}
	return reason, nil
}

func validateNote(note string) (string, error) {
	note = strings.TrimSpace(note)
	if len(note) > 200 {
		return "", fmt.Errorf("note must be at most 200 characters")
	}
	return note, nil
}
