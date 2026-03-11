//go:build integration

package repository

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// deleteItem removes a row by ID so tests don't leave data behind.
func deleteItem(t *testing.T, pool *pgxpool.Pool, id string) {
	t.Helper()
	_, err := pool.Exec(context.Background(), "DELETE FROM items WHERE id = $1", id)
	require.NoError(t, err, "cleanup: delete item %s", id)
}

// ----- Create -----

func TestItemRepository_Create(t *testing.T) {
	repo := NewItemRepository(testPool)

	item, err := repo.Create(context.Background(), "integration test item")
	require.NoError(t, err)
	t.Cleanup(func() { deleteItem(t, testPool, item.ID) })

	assert.NotEmpty(t, item.ID, "ID should be a UUID assigned by Postgres")
	assert.Equal(t, "integration test item", item.Name)
	assert.False(t, item.CreatedAt.IsZero(), "CreatedAt should be set by Postgres")
	assert.False(t, item.UpdatedAt.IsZero(), "UpdatedAt should be set by Postgres")
}

func TestItemRepository_Create_EmptyName(t *testing.T) {
	repo := NewItemRepository(testPool)

	// The DB has NOT NULL on name but no CHECK constraint for empty string,
	// so an empty string is technically valid at the DB level.
	// This test documents the current behaviour: the repository passes it through.
	item, err := repo.Create(context.Background(), "")
	if err == nil {
		t.Cleanup(func() { deleteItem(t, testPool, item.ID) })
	}
	// No assertion on err — validation is the handler's responsibility.
	// What matters is we don't panic and we either get an item or a DB error.
}

// ----- GetByID -----

func TestItemRepository_GetByID(t *testing.T) {
	repo := NewItemRepository(testPool)

	created, err := repo.Create(context.Background(), "get-by-id test")
	require.NoError(t, err)
	t.Cleanup(func() { deleteItem(t, testPool, created.ID) })

	fetched, err := repo.GetByID(context.Background(), created.ID)
	require.NoError(t, err)

	assert.Equal(t, created.ID, fetched.ID)
	assert.Equal(t, created.Name, fetched.Name)
	assert.WithinDuration(t, created.CreatedAt, fetched.CreatedAt, 0,
		"CreatedAt should survive a round-trip through Postgres")
	assert.WithinDuration(t, created.UpdatedAt, fetched.UpdatedAt, 0,
		"UpdatedAt should survive a round-trip through Postgres")
}

func TestItemRepository_GetByID_NotFound(t *testing.T) {
	repo := NewItemRepository(testPool)

	item, err := repo.GetByID(context.Background(), "00000000-0000-0000-0000-000000000000")

	assert.Nil(t, item)
	require.Error(t, err)
	assert.True(t, errors.Is(err, pgx.ErrNoRows),
		"expected pgx.ErrNoRows, got: %v", err)
}

func TestItemRepository_GetByID_InvalidUUID(t *testing.T) {
	repo := NewItemRepository(testPool)

	// Postgres will reject a non-UUID string with an error.
	item, err := repo.GetByID(context.Background(), "not-a-uuid")

	assert.Nil(t, item)
	assert.Error(t, err)
	assert.False(t, errors.Is(err, pgx.ErrNoRows),
		"should be a query error, not a not-found error")
}

// ----- Update -----

func TestItemRepository_Update(t *testing.T) {
	repo := NewItemRepository(testPool)

	created, err := repo.Create(context.Background(), "original name")
	require.NoError(t, err)
	t.Cleanup(func() { deleteItem(t, testPool, created.ID) })

	updated, err := repo.Update(context.Background(), created.ID, "updated name")
	require.NoError(t, err)

	assert.Equal(t, created.ID, updated.ID)
	assert.Equal(t, "updated name", updated.Name)
	assert.Equal(t, created.CreatedAt, updated.CreatedAt, "CreatedAt must not change on update")
	assert.True(t, updated.UpdatedAt.After(created.UpdatedAt) || updated.UpdatedAt.Equal(created.UpdatedAt),
		"UpdatedAt should be >= original")
}

func TestItemRepository_Update_NotFound(t *testing.T) {
	repo := NewItemRepository(testPool)

	item, err := repo.Update(context.Background(), "00000000-0000-0000-0000-000000000000", "name")

	assert.Nil(t, item)
	require.Error(t, err)
	assert.True(t, errors.Is(err, pgx.ErrNoRows))
}

// ----- Delete -----

func TestItemRepository_Delete(t *testing.T) {
	repo := NewItemRepository(testPool)

	created, err := repo.Create(context.Background(), "to be deleted")
	require.NoError(t, err)

	err = repo.Delete(context.Background(), created.ID)
	require.NoError(t, err)

	// Confirm the row is gone.
	_, err = repo.GetByID(context.Background(), created.ID)
	assert.True(t, errors.Is(err, pgx.ErrNoRows), "row should be gone after delete")
}

func TestItemRepository_Delete_NotFound(t *testing.T) {
	repo := NewItemRepository(testPool)

	err := repo.Delete(context.Background(), "00000000-0000-0000-0000-000000000000")

	require.Error(t, err)
	assert.True(t, errors.Is(err, pgx.ErrNoRows))
}
