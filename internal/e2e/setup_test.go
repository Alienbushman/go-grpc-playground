//go:build e2e

// Package e2e contains end-to-end tests that exercise the full stack:
// a real gRPC server, a real HTTP/JSON gateway (with swagger), and a real
// PostgreSQL database. No mocks are used at any layer.
//
// Run with: go test -tags=e2e ./internal/e2e/...
// Or via mage: mage testE2E
package e2e

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/grpc-ecosystem/grpc-gateway/v2/runtime"
	"github.com/jackc/pgx/v5/pgxpool"
	tc "github.com/testcontainers/testcontainers-go"
	pgmodule "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

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

	stopServers, err := startServers(ctx, pool)
	if err != nil {
		pool.Close()
		if pgc != nil {
			pgc.Terminate(ctx) //nolint:errcheck
		}
		fmt.Fprintf(os.Stderr, "e2e: start servers: %v\n", err)
		return 1
	}

	code := m.Run()

	stopServers()
	pool.Close()
	if pgc != nil {
		if err := pgc.Terminate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "e2e: terminate container: %v\n", err)
		}
	}
	return code
}

// startServers wires the real dependency graph, starts a gRPC server and an
// HTTP/JSON gateway (including the swagger endpoint) on random localhost ports.
// Sets the package-level grpcAddr. Returns a stop function.
func startServers(ctx context.Context, pool *pgxpool.Pool) (stop func(), err error) {
	itemRepo := repository.NewItemRepository(pool)
	itemSrv := server.NewItemServer(itemRepo)

	// gRPC server on a random port.
	grpcLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("listen grpc: %w", err)
	}
	grpcServer := grpc.NewServer(
		grpc.UnaryInterceptor(server.UnaryLoggingInterceptor),
	)
	itemv1.RegisterItemServiceServer(grpcServer, itemSrv)
	go func() {
		if err := grpcServer.Serve(grpcLis); err != nil && !errors.Is(err, grpc.ErrServerStopped) {
			fmt.Fprintf(os.Stderr, "e2e grpc server: %v\n", err)
		}
	}()
	grpcAddr = grpcLis.Addr().String()

	// HTTP/JSON gateway on a separate random port, mirroring the production
	// setup in main.go — including the swagger endpoint.
	mux := runtime.NewServeMux()
	mux.HandlePath("GET", "/swagger.json", func(w http.ResponseWriter, r *http.Request, _ map[string]string) {
		// Path is relative to the package directory (internal/e2e/).
		http.ServeFile(w, r, "../../gen/item/item.swagger.json")
	})

	dialOpts := []grpc.DialOption{grpc.WithTransportCredentials(insecure.NewCredentials())}
	if err := itemv1.RegisterItemServiceHandlerFromEndpoint(ctx, mux, grpcAddr, dialOpts); err != nil {
		grpcServer.Stop()
		return nil, fmt.Errorf("register gateway: %w", err)
	}

	httpLis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		grpcServer.Stop()
		return nil, fmt.Errorf("listen http: %w", err)
	}
	httpServer := &http.Server{Handler: mux}
	go func() {
		if err := httpServer.Serve(httpLis); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(os.Stderr, "e2e http server: %v\n", err)
		}
	}()

	return func() {
		grpcServer.GracefulStop()
		httpServer.Shutdown(context.Background()) //nolint:errcheck
	}, nil
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
