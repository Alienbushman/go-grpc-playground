package repository

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Item is the domain model for an item record.
type Item struct {
	ID        string
	Name      string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ItemRepository defines all database operations for items.
//
//go:generate mockery --name=ItemRepository
type ItemRepository interface {
	Create(ctx context.Context, name string) (*Item, error)
	GetByID(ctx context.Context, id string) (*Item, error)
	Update(ctx context.Context, id string, name string) (*Item, error)
	Delete(ctx context.Context, id string) error
}

type pgxItemRepository struct {
	pool *pgxpool.Pool
}

// NewItemRepository returns a PostgreSQL-backed ItemRepository.
func NewItemRepository(pool *pgxpool.Pool) ItemRepository {
	return &pgxItemRepository{pool: pool}
}

func (r *pgxItemRepository) Create(ctx context.Context, name string) (*Item, error) {
	const q = `
		INSERT INTO items (name)
		VALUES ($1)
		RETURNING id, name, created_at, updated_at`

	item := &Item{}
	err := r.pool.QueryRow(ctx, q, name).Scan(
		&item.ID,
		&item.Name,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r *pgxItemRepository) GetByID(ctx context.Context, id string) (*Item, error) {
	const q = `
		SELECT id, name, created_at, updated_at
		FROM items
		WHERE id = $1`

	item := &Item{}
	err := r.pool.QueryRow(ctx, q, id).Scan(
		&item.ID,
		&item.Name,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r *pgxItemRepository) Update(ctx context.Context, id string, name string) (*Item, error) {
	const q = `
		UPDATE items
		SET name = $2, updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, created_at, updated_at`

	item := &Item{}
	err := r.pool.QueryRow(ctx, q, id, name).Scan(
		&item.ID,
		&item.Name,
		&item.CreatedAt,
		&item.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return item, nil
}

func (r *pgxItemRepository) Delete(ctx context.Context, id string) error {
	const q = `DELETE FROM items WHERE id = $1`

	tag, err := r.pool.Exec(ctx, q, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}
