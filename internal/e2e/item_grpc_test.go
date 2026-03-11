//go:build e2e

package e2e

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"

	itemv1 "github.com/Alienbushman/go-grpc-playground/gen/item"
)

// newGRPCClient dials the test gRPC server and registers cleanup on t.
func newGRPCClient(t *testing.T) itemv1.ItemServiceClient {
	t.Helper()
	conn, err := grpc.NewClient(grpcAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	t.Cleanup(func() { conn.Close() })
	return itemv1.NewItemServiceClient(conn)
}

// TestItemGRPC_FullLifecycle exercises the complete CRUD flow over gRPC against
// a real database. It creates, reads, updates, deletes, then confirms deletion.
func TestItemGRPC_FullLifecycle(t *testing.T) {
	ctx := context.Background()
	client := newGRPCClient(t)

	// Create
	createResp, err := client.CreateItem(ctx, &itemv1.CreateItemRequest{Name: "grpc e2e item"})
	require.NoError(t, err)
	require.NotNil(t, createResp.Item)
	id := createResp.Item.Id
	assert.NotEmpty(t, id)
	assert.Equal(t, "grpc e2e item", createResp.Item.Name)
	assert.NotEmpty(t, createResp.Item.CreatedAt)
	assert.NotEmpty(t, createResp.Item.UpdatedAt)

	// Get — confirm the row is in the DB
	getResp, err := client.GetItem(ctx, &itemv1.GetItemRequest{Id: id})
	require.NoError(t, err)
	require.NotNil(t, getResp.Item)
	assert.Equal(t, id, getResp.Item.Id)
	assert.Equal(t, "grpc e2e item", getResp.Item.Name)

	// Update
	updateResp, err := client.UpdateItem(ctx, &itemv1.UpdateItemRequest{
		Id:   id,
		Name: "grpc e2e item updated",
	})
	require.NoError(t, err)
	require.NotNil(t, updateResp.Item)
	assert.Equal(t, id, updateResp.Item.Id)
	assert.Equal(t, "grpc e2e item updated", updateResp.Item.Name)

	// Delete
	_, err = client.DeleteItem(ctx, &itemv1.DeleteItemRequest{Id: id})
	require.NoError(t, err)

	// Confirm deletion — GetItem must return NotFound
	_, err = client.GetItem(ctx, &itemv1.GetItemRequest{Id: id})
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

// ----- Validation errors -----

func TestItemGRPC_CreateItem_EmptyName(t *testing.T) {
	client := newGRPCClient(t)

	resp, err := client.CreateItem(context.Background(), &itemv1.CreateItemRequest{Name: ""})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
	assert.Contains(t, status.Convert(err).Message(), "name is required")
}

func TestItemGRPC_GetItem_EmptyID(t *testing.T) {
	client := newGRPCClient(t)

	resp, err := client.GetItem(context.Background(), &itemv1.GetItemRequest{Id: ""})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.InvalidArgument, status.Code(err))
}

// ----- Not found -----

func TestItemGRPC_GetItem_NotFound(t *testing.T) {
	client := newGRPCClient(t)

	resp, err := client.GetItem(context.Background(), &itemv1.GetItemRequest{
		Id: "00000000-0000-0000-0000-000000000000",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestItemGRPC_UpdateItem_NotFound(t *testing.T) {
	client := newGRPCClient(t)

	resp, err := client.UpdateItem(context.Background(), &itemv1.UpdateItemRequest{
		Id:   "00000000-0000-0000-0000-000000000000",
		Name: "name",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}

func TestItemGRPC_DeleteItem_NotFound(t *testing.T) {
	client := newGRPCClient(t)

	resp, err := client.DeleteItem(context.Background(), &itemv1.DeleteItemRequest{
		Id: "00000000-0000-0000-0000-000000000000",
	})

	assert.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, codes.NotFound, status.Code(err))
}
