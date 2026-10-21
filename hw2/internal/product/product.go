// Package product содержит доменную модель товара и работу с БД.
package product

import (
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
)

type Status string

const (
	StatusActive   Status = "ACTIVE"
	StatusInactive Status = "INACTIVE"
	StatusArchived Status = "ARCHIVED"
)

// ErrNotFound — товар с указанным ID не существует.
var ErrNotFound = errors.New("product not found")

// Product — товар в том виде, в котором он хранится в БД.
type Product struct {
	ID          uuid.UUID
	Name        string
	Description *string
	Price       decimal.Decimal
	Stock       int32
	Category    string
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// Fields — изменяемые поля товара (для создания и полного обновления).
type Fields struct {
	Name        string
	Description *string
	Price       decimal.Decimal
	Stock       int32
	Category    string
	Status      Status
}

// ListFilter — фильтры и пагинация для списка товаров.
// Nil-фильтр означает «не фильтровать».
type ListFilter struct {
	Status   *Status
	Category *string
	Limit    int64
	Offset   int64
}
