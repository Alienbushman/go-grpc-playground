package server

import (
	"context"
	"errors"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	itemv1 "github.com/rick/grpc-go-experimentation/gen/item"
	"github.com/rick/grpc-go-experimentation/internal/repository"
)

// ItemServer implements itemv1.ItemServiceServer.
type ItemServer struct {
	itemv1.UnimplementedItemServiceServer
	repo repository.ItemRepository
}

// NewItemServer creates an ItemServer backed by the given repository.
func NewItemServer(repo repository.ItemRepository) *ItemServer {
	return &ItemServer{repo: repo}
}

func (s *ItemServer) CreateItem(ctx context.Context, req *itemv1.CreateItemRequest) (*itemv1.CreateItemResponse, error) {
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	item, err := s.repo.Create(ctx, req.Name)
	if err != nil {
		slog.Error("failed to create item", "error", err)
		return nil, status.Errorf(codes.Internal, "create item: %v", err)
	}

	slog.Info("item created", "id", item.ID, "name", item.Name)
	return &itemv1.CreateItemResponse{Item: toProto(item)}, nil
}

func (s *ItemServer) GetItem(ctx context.Context, req *itemv1.GetItemRequest) (*itemv1.GetItemResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	item, err := s.repo.GetByID(ctx, req.Id)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "item %q not found", req.Id)
		}
		slog.Error("failed to get item", "id", req.Id, "error", err)
		return nil, status.Errorf(codes.Internal, "get item: %v", err)
	}

	return &itemv1.GetItemResponse{Item: toProto(item)}, nil
}

func (s *ItemServer) UpdateItem(ctx context.Context, req *itemv1.UpdateItemRequest) (*itemv1.UpdateItemResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}
	if req.Name == "" {
		return nil, status.Error(codes.InvalidArgument, "name is required")
	}

	item, err := s.repo.Update(ctx, req.Id, req.Name)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "item %q not found", req.Id)
		}
		slog.Error("failed to update item", "id", req.Id, "error", err)
		return nil, status.Errorf(codes.Internal, "update item: %v", err)
	}

	slog.Info("item updated", "id", item.ID, "name", item.Name)
	return &itemv1.UpdateItemResponse{Item: toProto(item)}, nil
}

func (s *ItemServer) DeleteItem(ctx context.Context, req *itemv1.DeleteItemRequest) (*itemv1.DeleteItemResponse, error) {
	if req.Id == "" {
		return nil, status.Error(codes.InvalidArgument, "id is required")
	}

	if err := s.repo.Delete(ctx, req.Id); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, status.Errorf(codes.NotFound, "item %q not found", req.Id)
		}
		slog.Error("failed to delete item", "id", req.Id, "error", err)
		return nil, status.Errorf(codes.Internal, "delete item: %v", err)
	}

	slog.Info("item deleted", "id", req.Id)
	return &itemv1.DeleteItemResponse{}, nil
}

// toProto maps a repository.Item to the proto-generated Item type.
func toProto(i *repository.Item) *itemv1.Item {
	return &itemv1.Item{
		Id:        i.ID,
		Name:      i.Name,
		CreatedAt: i.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt: i.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
	}
}
