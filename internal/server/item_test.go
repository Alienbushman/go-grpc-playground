package server

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	itemv1 "github.com/rick/grpc-go-experimentation/gen/item"
	"github.com/rick/grpc-go-experimentation/internal/repository"
)

// mockItemRepo is a hand-written test double for ItemRepository.
// Set only the function fields exercised by each test case.
type mockItemRepo struct {
	createFn  func(ctx context.Context, name string) (*repository.Item, error)
	getByIDFn func(ctx context.Context, id string) (*repository.Item, error)
	updateFn  func(ctx context.Context, id string, name string) (*repository.Item, error)
	deleteFn  func(ctx context.Context, id string) error
}

func (m *mockItemRepo) Create(ctx context.Context, name string) (*repository.Item, error) {
	return m.createFn(ctx, name)
}

func (m *mockItemRepo) GetByID(ctx context.Context, id string) (*repository.Item, error) {
	return m.getByIDFn(ctx, id)
}

func (m *mockItemRepo) Update(ctx context.Context, id string, name string) (*repository.Item, error) {
	return m.updateFn(ctx, id, name)
}

func (m *mockItemRepo) Delete(ctx context.Context, id string) error {
	return m.deleteFn(ctx, id)
}

// fixedTime is a stable timestamp used across mapping tests.
var fixedTime = time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)

// fixedItem is a repository.Item with all fields set to known values.
func fixedItem() *repository.Item {
	return &repository.Item{
		ID:        "abc-123",
		Name:      "test item",
		CreatedAt: fixedTime,
		UpdatedAt: fixedTime,
	}
}

// ----- CreateItem -----

func TestItemServer_CreateItem(t *testing.T) {
	tests := []struct {
		name     string
		req      *itemv1.CreateItemRequest
		repoFn   func(ctx context.Context, n string) (*repository.Item, error)
		wantCode codes.Code
		check    func(t *testing.T, resp *itemv1.CreateItemResponse)
	}{
		{
			name: "creates item and returns it",
			req:  &itemv1.CreateItemRequest{Name: "test item"},
			repoFn: func(_ context.Context, name string) (*repository.Item, error) {
				return &repository.Item{
					ID:        "abc-123",
					Name:      name,
					CreatedAt: fixedTime,
					UpdatedAt: fixedTime,
				}, nil
			},
			wantCode: codes.OK,
			check: func(t *testing.T, resp *itemv1.CreateItemResponse) {
				require.NotNil(t, resp.Item)
				assert.Equal(t, "abc-123", resp.Item.Id)
				assert.Equal(t, "test item", resp.Item.Name)
				assert.NotEmpty(t, resp.Item.CreatedAt)
				assert.NotEmpty(t, resp.Item.UpdatedAt)
			},
		},
		{
			name:     "returns InvalidArgument when name is empty",
			req:      &itemv1.CreateItemRequest{Name: ""},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "returns Internal when repo fails",
			req:  &itemv1.CreateItemRequest{Name: "boom"},
			repoFn: func(_ context.Context, _ string) (*repository.Item, error) {
				return nil, errors.New("db is down")
			},
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewItemServer(&mockItemRepo{createFn: tt.repoFn})
			resp, err := srv.CreateItem(context.Background(), tt.req)

			assert.Equal(t, tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, resp)
				}
			} else {
				require.Error(t, err)
				assert.Nil(t, resp)
			}
		})
	}
}

// ----- GetItem -----

func TestItemServer_GetItem(t *testing.T) {
	tests := []struct {
		name     string
		req      *itemv1.GetItemRequest
		repoFn   func(ctx context.Context, id string) (*repository.Item, error)
		wantCode codes.Code
		check    func(t *testing.T, resp *itemv1.GetItemResponse)
	}{
		{
			name: "returns item when found",
			req:  &itemv1.GetItemRequest{Id: "abc-123"},
			repoFn: func(_ context.Context, id string) (*repository.Item, error) {
				item := fixedItem()
				item.ID = id
				return item, nil
			},
			wantCode: codes.OK,
			check: func(t *testing.T, resp *itemv1.GetItemResponse) {
				require.NotNil(t, resp.Item)
				assert.Equal(t, "abc-123", resp.Item.Id)
				assert.Equal(t, "test item", resp.Item.Name)
			},
		},
		{
			name:     "returns InvalidArgument when id is empty",
			req:      &itemv1.GetItemRequest{Id: ""},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "returns NotFound when row does not exist",
			req:  &itemv1.GetItemRequest{Id: "missing"},
			repoFn: func(_ context.Context, _ string) (*repository.Item, error) {
				return nil, pgx.ErrNoRows
			},
			wantCode: codes.NotFound,
		},
		{
			name: "returns Internal on unexpected repo error",
			req:  &itemv1.GetItemRequest{Id: "abc-123"},
			repoFn: func(_ context.Context, _ string) (*repository.Item, error) {
				return nil, errors.New("connection reset")
			},
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewItemServer(&mockItemRepo{getByIDFn: tt.repoFn})
			resp, err := srv.GetItem(context.Background(), tt.req)

			assert.Equal(t, tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, resp)
				}
			} else {
				require.Error(t, err)
				assert.Nil(t, resp)
			}
		})
	}
}

// ----- UpdateItem -----

func TestItemServer_UpdateItem(t *testing.T) {
	tests := []struct {
		name     string
		req      *itemv1.UpdateItemRequest
		repoFn   func(ctx context.Context, id, n string) (*repository.Item, error)
		wantCode codes.Code
		check    func(t *testing.T, resp *itemv1.UpdateItemResponse)
	}{
		{
			name: "updates item and returns it",
			req:  &itemv1.UpdateItemRequest{Id: "abc-123", Name: "updated name"},
			repoFn: func(_ context.Context, id, name string) (*repository.Item, error) {
				item := fixedItem()
				item.ID = id
				item.Name = name
				return item, nil
			},
			wantCode: codes.OK,
			check: func(t *testing.T, resp *itemv1.UpdateItemResponse) {
				require.NotNil(t, resp.Item)
				assert.Equal(t, "abc-123", resp.Item.Id)
				assert.Equal(t, "updated name", resp.Item.Name)
			},
		},
		{
			name:     "returns InvalidArgument when id is empty",
			req:      &itemv1.UpdateItemRequest{Id: "", Name: "name"},
			wantCode: codes.InvalidArgument,
		},
		{
			name:     "returns InvalidArgument when name is empty",
			req:      &itemv1.UpdateItemRequest{Id: "abc-123", Name: ""},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "returns NotFound when item does not exist",
			req:  &itemv1.UpdateItemRequest{Id: "missing", Name: "name"},
			repoFn: func(_ context.Context, _, _ string) (*repository.Item, error) {
				return nil, pgx.ErrNoRows
			},
			wantCode: codes.NotFound,
		},
		{
			name: "returns Internal on unexpected repo error",
			req:  &itemv1.UpdateItemRequest{Id: "abc-123", Name: "name"},
			repoFn: func(_ context.Context, _, _ string) (*repository.Item, error) {
				return nil, errors.New("db is down")
			},
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewItemServer(&mockItemRepo{updateFn: tt.repoFn})
			resp, err := srv.UpdateItem(context.Background(), tt.req)

			assert.Equal(t, tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, resp)
				}
			} else {
				require.Error(t, err)
				assert.Nil(t, resp)
			}
		})
	}
}

// ----- DeleteItem -----

func TestItemServer_DeleteItem(t *testing.T) {
	tests := []struct {
		name     string
		req      *itemv1.DeleteItemRequest
		repoFn   func(ctx context.Context, id string) error
		wantCode codes.Code
	}{
		{
			name:     "deletes item successfully",
			req:      &itemv1.DeleteItemRequest{Id: "abc-123"},
			repoFn:   func(_ context.Context, _ string) error { return nil },
			wantCode: codes.OK,
		},
		{
			name:     "returns InvalidArgument when id is empty",
			req:      &itemv1.DeleteItemRequest{Id: ""},
			wantCode: codes.InvalidArgument,
		},
		{
			name: "returns NotFound when item does not exist",
			req:  &itemv1.DeleteItemRequest{Id: "missing"},
			repoFn: func(_ context.Context, _ string) error {
				return pgx.ErrNoRows
			},
			wantCode: codes.NotFound,
		},
		{
			name: "returns Internal on unexpected repo error",
			req:  &itemv1.DeleteItemRequest{Id: "abc-123"},
			repoFn: func(_ context.Context, _ string) error {
				return errors.New("db is down")
			},
			wantCode: codes.Internal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := NewItemServer(&mockItemRepo{deleteFn: tt.repoFn})
			resp, err := srv.DeleteItem(context.Background(), tt.req)

			assert.Equal(t, tt.wantCode, status.Code(err))
			if tt.wantCode == codes.OK {
				require.NoError(t, err)
				assert.NotNil(t, resp)
			} else {
				require.Error(t, err)
				assert.Nil(t, resp)
			}
		})
	}
}

// ----- toProto -----

func TestToProto(t *testing.T) {
	item := fixedItem()
	proto := toProto(item)

	assert.Equal(t, item.ID, proto.Id)
	assert.Equal(t, item.Name, proto.Name)
	assert.Equal(t, "2026-03-10T12:00:00Z", proto.CreatedAt)
	assert.Equal(t, "2026-03-10T12:00:00Z", proto.UpdatedAt)
}
