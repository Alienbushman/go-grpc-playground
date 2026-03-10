package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/reflection"

	itemv1 "github.com/Alienbushman/go-grpc-playground/gen/item"
	"github.com/Alienbushman/go-grpc-playground/internal/repository"
	"github.com/Alienbushman/go-grpc-playground/internal/server"
)

func main() {
	dsn := mustEnv("DATABASE_URL")
	grpcPort := envOrDefault("GRPC_PORT", "50051")
	httpPort := envOrDefault("HTTP_PORT", "8080")

	// Connect to Postgres
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		slog.Error("failed to create db pool", "error", err)
		os.Exit(1)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		slog.Error("failed to ping db", "error", err)
		os.Exit(1)
	}
	slog.Info("connected to database")

	// Wire dependencies
	itemRepo := repository.NewItemRepository(pool)
	itemSrv := server.NewItemServer(itemRepo)

	// Start gRPC server
	lis, err := net.Listen("tcp", fmt.Sprintf(":%s", grpcPort))
	if err != nil {
		slog.Error("failed to listen", "port", grpcPort, "error", err)
		os.Exit(1)
	}

	grpcServer := grpc.NewServer()
	itemv1.RegisterItemServiceServer(grpcServer, itemSrv)
	reflection.Register(grpcServer) // enables grpcurl without a proto file

	go func() {
		slog.Info("gRPC server listening", "port", grpcPort)
		if err := grpcServer.Serve(lis); err != nil {
			slog.Error("gRPC server stopped unexpectedly", "error", err)
		}
	}()

	// Start HTTP/JSON gateway
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	mux := runtime.NewServeMux()
	// serve swagger/openapi JSON alongside the API if the file exists
	mux.HandlePath("GET", "/swagger.json", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		http.ServeFile(w, r, "gen/item/item.swagger.json")
	})

	opts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if err := itemv1.RegisterItemServiceHandlerFromEndpoint(ctx, mux,
		fmt.Sprintf("localhost:%s", grpcPort), opts,
	); err != nil {
		slog.Error("failed to register gateway", "error", err)
		os.Exit(1)
	}

	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%s", httpPort),
		Handler: mux,
	}

	go func() {
		slog.Info("HTTP gateway listening", "port", httpPort)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP gateway stopped unexpectedly", "error", err)
		}
	}()

	// Block until SIGINT or SIGTERM, then shut down gracefully
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	slog.Info("shutting down servers...")
	grpcServer.GracefulStop()
	if err := httpServer.Shutdown(context.Background()); err != nil {
		slog.Error("HTTP gateway shutdown error", "error", err)
	}
	slog.Info("servers stopped")
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		slog.Error("required environment variable not set", "key", key)
		os.Exit(1)
	}
	return v
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
