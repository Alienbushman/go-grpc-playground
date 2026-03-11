//go:build e2e

// Package e2e contains end-to-end tests that exercise the full gRPC stack:
// a real gRPC server and a real PostgreSQL database. No mocks are used.
//
// Run with: go test -tags=e2e ./internal/e2e/...
// Or via mage: mage testE2E
package e2e

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tc "github.com/testcontainers/testcontainers-go"
	pgmodule "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"

	itemv1 "github.com/Alienbushman/go-grpc-playground/gen/item"
	"github.com/Alienbushman/go-grpc-playground/internal/repository"
	"github.com/Alienbushman/go-grpc-playground/internal/server"
)

// grpcAddr is set by TestMain and read (never written) by tests.
var grpcAddr string // host:port — e.g. "127.0.0.1:52341"

func TestMain(m *testing.M) {
	os.Exit(runE2E(m))
}

// runE2E sets up the full stack, runs the suite, and tears down.
// Priority for the database:
//  1. DATABASE_URL env var (e.g. CI with a pre-existing DB)
//  2. Postgres testcontainer (automatic — requires Docker)
//
// If Docker is unavailable and DATABASE_URL is unset the suite exits 0 so
// that `go test -tags=e2e ./...` does not fail in environments without Docker.
func runE2E(m *testing.M) int {
	ctx := context.Background()

	dsn := os.Getenv("DATABASE_URL")
	var pgc *pgmodule.PostgresContainer

	if dsn == "" {
		var err error
		pgc, err = pgmodule.Run(ctx,
			"postgres:16-alpine",
			pgmodule.WithDatabase("grpc_experiment"),
			pgmodule.WithUsername("grpc"),
			pgmodule.WithPassword("grpc"),
			tc.WithWaitStrategy(
				wait.ForLog("database system is ready to accept connections").
					WithOccurrence(2).
					WithStartupTimeout(30*time.Second),
			),
		)
		if err != nil {
			fmt.Fprintf(os.Stderr, "e2e: skipping — could not start postgres container: %v\n", err)
			return 0
		}

		dsn, err = pgc.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			pgc.Terminate(ctx) //nolint:errcheck
			fmt.Fprintf(os.Stderr, "e2e: connection string: %v\n", err)
			return 1
		}
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		if pgc != nil {
			pgc.Terminate(ctx) //nolint:errcheck
		}
		fmt.Fprintf(os.Stderr, "e2e: open pool: %v\n", err)
		return 1
	}

	if err := applyMigrations(ctx, pool); err != nil {
		pool.Close()
		if pgc != nil {
			pgc.Terminate(ctx) //nolint:errcheck
		}
		fmt.Fprintf(os.Stderr, "e2e: apply migrations: %v\n", err)
		return 1
	}

	stopServer, err := startGRPCServer(pool)
	if err != nil {
		pool.Close()
		if pgc != nil {
			pgc.Terminate(ctx) //nolint:errcheck
		}
		fmt.Fprintf(os.Stderr, "e2e: start server: %v\n", err)
		return 1
	}

	code := m.Run()

	stopServer()
	pool.Close()
	if pgc != nil {
		if err := pgc.Terminate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: terminate container: %v\n", err)
		}
	}
	return code
}

// startGRPCServer wires the real dependency graph and starts a gRPC server on
// a random localhost port. Sets the package-level grpcAddr. Returns a stop func.
func startGRPCServer(pool *pgxpool.Pool) (stop func(), err error) {
	itemRepo := repository.NewItemRepository(pool)
	itemSrv := server.NewItemServer(itemRepo)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen: %w", err)
	}

	grpcServer := grpc.NewServer()
	itemv1.RegisterItemServiceServer(grpcServer, itemSrv)

	go func() {
		if err := grpcServer.Serve(lis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			fmt.Fprintf(os.Stderr, "e2e grpc server: %v\n", err)
		}
	}()

	grpcAddr = lis.Addr().String()
	return grpcServer.GracefulStop, nil
}

// applyMigrations runs all up-migrations against the pool.
// Path is relative to the package directory (internal/e2e/).
func applyMigrations(ctx context.Context, pool *pgxpool.Pool) error {
	sql, err := os.ReadFile("../../migrations/000001_create_items.up.sql")
	if err != nil {
		return fmt.Errorf("read migration: %w", err)
	}
	if _, err := pool.Exec(ctx, string(sql)); err != nil {
		return fmt.Errorf("exec migration: %w", err)
	}
	return nil
}
