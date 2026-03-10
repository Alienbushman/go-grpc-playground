// Package server_test contains gRPC integration tests for ItemService.
//
// Unlike the unit tests in item_test.go (which call handler methods directly),
// these tests exercise the full gRPC stack: request serialisation over an
// in-process bufconn transport, server-side handler execution, and response
// deserialisation back into proto types.
//
// The repository layer is replaced by a mockery-generated MockItemRepository
// so no database is required.
//
// Note: context arguments in EXPECT() use mock.Anything because the gRPC
// server enriches the incoming context with transport metadata (peer info,
// stream keys, etc.) before passing it to the handler.
package server_test

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	mockrepository "github.com/rick/grpc-go-experimentation/mocks/repository"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"

	itemv1 "github.com/rick/grpc-go-experimentation/gen/item"
	"github.com/rick/grpc-go-experimentation/internal/repository"
	"github.com/rick/grpc-go-experimentation/internal/server"
)

const bufSize = 1024 * 1024

// newTestServer starts a real gRPC server backed by the given repository mock,
// returns a connected client, and registers cleanup on t.
func newTestServer(t *testing.T, repo repository.ItemRepository) itemv1.ItemServiceClient {
	t.Helper()

	lis := bufconn.Listen(bufSize)
	grpcServer := grpc.NewServer()
	itemv1.RegisterItemServiceServer(grpcServer, server.NewItemServer(repo))

	go func() {
		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			t.Logf("grpc server error: %v", err)
		}
	}()

	t.Cleanup(func() {
		grpcServer.GracefulStop()
		lis.Close()
	})

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
			return lis.DialContext(ctx)
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })

	return itemv1.NewItemServiceClient(conn)
}

// fixedRepoItem is a known-good repository.Item used across test expectations.
func fixedRepoItem() *repository.Item {
	return &repository.Item{
		ID:        "9d69498d-c582-4c22-bbb4-658913dcab9a",
		Name:      "my first item",
		CreatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
		UpdatedAt: time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC),
	}
}

// ----- CreateItem -----

func TestItemService_CreateItem_Success(t *testing.T) {
	repo := mockrepository.NewMockItemRepository(t)
	repo.EXPECT().
		Create(mock.Anything, "my first item").
		Return(fixedRepoItem(), nil)

	client := newTestServer(t, repo)

	resp, err := client.CreateItem(context.Background(), &itemv1.CreateItemRequest{
		Name: "my first item",
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Item)
	assert.Equal(t, "9d69498d-c582-4c22-bbb4-658913dcab9a", resp.Item.Id)
	assert.Equal(t, "my first item", resp.Item.Name)
	assert.Equal(t, "2026-03-10T12:00:00Z", resp.Item.CreatedAt)
	assert.Equal(t, "2026-03-10T12:00:00Z", resp.Item.UpdatedAt)
}

func TestItemService_CreateItem_EmptyName(t *testing.T) {
	// Repository must never be called when input validation fails.
	repo := mockrepository.NewMockItemRepository(t)

	client := newTestServer(t, repo)

	resp, err := client.CreateItem(context.Background(), &itemv1.CreateItemRequest{Name: ""})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "name is required")
}

func TestItemService_CreateItem_RepoError(t *testing.T) {
	repo := mockrepository.NewMockItemRepository(t)
	repo.EXPECT().
		Create(mock.Anything, "boom").
		Return(nil, errors.New("db is down"))

	client := newTestServer(t, repo)

	resp, err := client.CreateItem(context.Background(), &itemv1.CreateItemRequest{Name: "boom"})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

// ----- GetItem -----

func TestItemService_GetItem_Success(t *testing.T) {
	id := "9d69498d-c582-4c22-bbb4-658913dcab9a"
	repo := mockrepository.NewMockItemRepository(t)
	repo.EXPECT().
		GetByID(mock.Anything, id).
		Return(fixedRepoItem(), nil)

	client := newTestServer(t, repo)

	resp, err := client.GetItem(context.Background(), &itemv1.GetItemRequest{Id: id})

	require.NoError(t, err)
	require.NotNil(t, resp.Item)
	assert.Equal(t, id, resp.Item.Id)
	assert.Equal(t, "my first item", resp.Item.Name)
	assert.Equal(t, "2026-03-10T12:00:00Z", resp.Item.CreatedAt)
}

func TestItemService_GetItem_EmptyID(t *testing.T) {
	// Repository must never be called when input validation fails.
	repo := mockrepository.NewMockItemRepository(t)

	client := newTestServer(t, repo)

	resp, err := client.GetItem(context.Background(), &itemv1.GetItemRequest{Id: ""})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "id is required")
}

func TestItemService_GetItem_NotFound(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000000"
	repo := mockrepository.NewMockItemRepository(t)
	repo.EXPECT().
		GetByID(mock.Anything, id).
		Return(nil, pgx.ErrNoRows)

	client := newTestServer(t, repo)

	resp, err := client.GetItem(context.Background(), &itemv1.GetItemRequest{Id: id})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), id)
}

func TestItemService_GetItem_RepoError(t *testing.T) {
	id := "9d69498d-c582-4c22-bbb4-658913dcab9a"
	repo := mockrepository.NewMockItemRepository(t)
	repo.EXPECT().
		GetByID(mock.Anything, id).
		Return(nil, errors.New("connection reset"))

	client := newTestServer(t, repo)

	resp, err := client.GetItem(context.Background(), &itemv1.GetItemRequest{Id: id})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

// ----- UpdateItem -----

func TestItemService_UpdateItem_Success(t *testing.T) {
	id := "9d69498d-c582-4c22-bbb4-658913dcab9a"
	updated := fixedRepoItem()
	updated.Name = "updated name"

	repo := mockrepository.NewMockItemRepository(t)
	repo.EXPECT().
		Update(mock.Anything, id, "updated name").
		Return(updated, nil)

	client := newTestServer(t, repo)

	resp, err := client.UpdateItem(context.Background(), &itemv1.UpdateItemRequest{
		Id: id, Name: "updated name",
	})

	require.NoError(t, err)
	require.NotNil(t, resp.Item)
	assert.Equal(t, id, resp.Item.Id)
	assert.Equal(t, "updated name", resp.Item.Name)
}

func TestItemService_UpdateItem_EmptyID(t *testing.T) {
	repo := mockrepository.NewMockItemRepository(t)

	client := newTestServer(t, repo)

	resp, err := client.UpdateItem(context.Background(), &itemv1.UpdateItemRequest{
		Id: "", Name: "name",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "id is required")
}

func TestItemService_UpdateItem_EmptyName(t *testing.T) {
	repo := mockrepository.NewMockItemRepository(t)

	client := newTestServer(t, repo)

	resp, err := client.UpdateItem(context.Background(), &itemv1.UpdateItemRequest{
		Id: "9d69498d-c582-4c22-bbb4-658913dcab9a", Name: "",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "name is required")
}

func TestItemService_UpdateItem_NotFound(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000000"
	repo := mockrepository.NewMockItemRepository(t)
	repo.EXPECT().
		Update(mock.Anything, id, "name").
		Return(nil, pgx.ErrNoRows)

	client := newTestServer(t, repo)

	resp, err := client.UpdateItem(context.Background(), &itemv1.UpdateItemRequest{
		Id: id, Name: "name",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), id)
}

func TestItemService_UpdateItem_RepoError(t *testing.T) {
	id := "9d69498d-c582-4c22-bbb4-658913dcab9a"
	repo := mockrepository.NewMockItemRepository(t)
	repo.EXPECT().
		Update(mock.Anything, id, "name").
		Return(nil, errors.New("db is down"))

	client := newTestServer(t, repo)

	resp, err := client.UpdateItem(context.Background(), &itemv1.UpdateItemRequest{
		Id: id, Name: "name",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}

// ----- DeleteItem -----

func TestItemService_DeleteItem_Success(t *testing.T) {
	id := "9d69498d-c582-4c22-bbb4-658913dcab9a"
	repo := mockrepository.NewMockItemRepository(t)
	repo.EXPECT().
		Delete(mock.Anything, id).
		Return(nil)

	client := newTestServer(t, repo)

	resp, err := client.DeleteItem(context.Background(), &itemv1.DeleteItemRequest{Id: id})

	require.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestItemService_DeleteItem_EmptyID(t *testing.T) {
	repo := mockrepository.NewMockItemRepository(t)

	client := newTestServer(t, repo)

	resp, err := client.DeleteItem(context.Background(), &itemv1.DeleteItemRequest{Id: ""})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "id is required")
}

func TestItemService_DeleteItem_NotFound(t *testing.T) {
	id := "00000000-0000-0000-0000-000000000000"
	repo := mockrepository.NewMockItemRepository(t)
	repo.EXPECT().
		Delete(mock.Anything, id).
		Return(pgx.ErrNoRows)

	client := newTestServer(t, repo)

	resp, err := client.DeleteItem(context.Background(), &itemv1.DeleteItemRequest{Id: id})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), id)
}

func TestItemService_DeleteItem_RepoError(t *testing.T) {
	id := "9d69498d-c582-4c22-bbb4-658913dcab9a"
	repo := mockrepository.NewMockItemRepository(t)
	repo.EXPECT().
		Delete(mock.Anything, id).
		Return(errors.New("db is down"))

	client := newTestServer(t, repo)

	resp, err := client.DeleteItem(context.Background(), &itemv1.DeleteItemRequest{Id: id})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.Internal, status.Code(err))
}
