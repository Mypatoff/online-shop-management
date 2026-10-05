package store

import "errors"

var (
	ErrNotFound          = errors.New("not found")
	ErrDuplicateSKU      = errors.New("sku already in use")
	ErrInsufficientStock = errors.New("insufficient stock")
	ErrArchived          = errors.New("product is archived")
	ErrAlreadyVoided     = errors.New("sale already voided")
)
