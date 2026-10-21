package product

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Repository выполняет SQL-запросы к таблице products.
type Repository struct {
	db *pgxpool.Pool
}

func NewRepository(db *pgxpool.Pool) *Repository {
	return &Repository{db: db}
}

// Статус передаётся как text и приводится к enum на стороне PostgreSQL:
// так драйверу не нужно знать о типе product_status, а индекс по status
// продолжает использоваться.
const columns = `id, name, description, price, stock, category, status::text, created_at, updated_at`

func (r *Repository) Create(ctx context.Context, f Fields) (Product, error) {
	row := r.db.QueryRow(ctx, `
		INSERT INTO products (name, description, price, stock, category, status)
		VALUES ($1, $2, $3, $4, $5, $6::text::product_status)
		RETURNING `+columns,
		f.Name, f.Description, f.Price, f.Stock, f.Category, string(f.Status))
	p, err := scan(row)
	if err != nil {
		return Product{}, fmt.Errorf("insert product: %w", err)
	}
	return p, nil
}

func (r *Repository) Get(ctx context.Context, id uuid.UUID) (Product, error) {
	row := r.db.QueryRow(ctx, `SELECT `+columns+` FROM products WHERE id = $1`, id)
	p, err := scan(row)
	if err != nil {
		return Product{}, notFoundOr(err, "get product")
	}
	return p, nil
}

// List возвращает страницу товаров и общее число товаров под фильтром.
func (r *Repository) List(ctx context.Context, f ListFilter) ([]Product, int64, error) {
	var status *string
	if f.Status != nil {
		s := string(*f.Status)
		status = &s
	}
	const where = `
		WHERE ($1::text IS NULL OR status = $1::text::product_status)
		  AND ($2::text IS NULL OR category = $2::text)`

	var total int64
	if err := r.db.QueryRow(ctx, `SELECT count(*) FROM products`+where, status, f.Category).Scan(&total); err != nil {
		return nil, 0, fmt.Errorf("count products: %w", err)
	}

	rows, err := r.db.Query(ctx, `SELECT `+columns+` FROM products`+where+`
		ORDER BY created_at DESC, id
		LIMIT $3 OFFSET $4`,
		status, f.Category, f.Limit, f.Offset)
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	products, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Product, error) { return scan(row) })
	if err != nil {
		return nil, 0, fmt.Errorf("list products: %w", err)
	}
	return products, total, nil
}

// Update полностью заменяет изменяемые поля товара.
// updated_at обновляет триггер в БД.
func (r *Repository) Update(ctx context.Context, id uuid.UUID, f Fields) (Product, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE products
		SET name = $2, description = $3, price = $4, stock = $5, category = $6,
		    status = $7::text::product_status
		WHERE id = $1
		RETURNING `+columns,
		id, f.Name, f.Description, f.Price, f.Stock, f.Category, string(f.Status))
	p, err := scan(row)
	if err != nil {
		return Product{}, notFoundOr(err, "update product")
	}
	return p, nil
}

// Archive — мягкое удаление: переводит товар в ARCHIVED.
// Уже архивный товар не изменяется (и его updated_at тоже),
// поэтому повторный вызов возвращает то же состояние.
func (r *Repository) Archive(ctx context.Context, id uuid.UUID) (Product, error) {
	row := r.db.QueryRow(ctx, `
		UPDATE products SET status = 'ARCHIVED'
		WHERE id = $1 AND status <> 'ARCHIVED'
		RETURNING `+columns, id)
	p, err := scan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		// Товара нет или он уже в архиве — Get различит эти случаи.
		return r.Get(ctx, id)
	}
	if err != nil {
		return Product{}, fmt.Errorf("archive product: %w", err)
	}
	return p, nil
}

func scan(row pgx.Row) (Product, error) {
	var p Product
	var status string
	err := row.Scan(&p.ID, &p.Name, &p.Description, &p.Price, &p.Stock,
		&p.Category, &status, &p.CreatedAt, &p.UpdatedAt)
	p.Status = Status(status)
	return p, err
}

func notFoundOr(err error, op string) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	return fmt.Errorf("%s: %w", op, err)
}
