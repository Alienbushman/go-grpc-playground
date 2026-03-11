//go:build integration

package repository

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	tc "github.com/testcontainers/testcontainers-go"
	pgmodule "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

// testPool is the shared connection pool used by all repository tests in this
// package. It is initialised once in TestMain before any test runs.
var testPool *pgxpool.Pool

func TestMain(m *testing.M) {
	os.Exit(runTests(m))
}

// runTests wires up a database, runs the test suite, and tears down cleanly.
// Priority:
//  1. DATABASE_URL env var (e.g. CI or manual local run)
//  2. Testcontainer (automatic Postgres in Docker)
//
// If Docker is unavailable and DATABASE_URL is unset the suite exits 0 so that
// `go test ./...` does not mark the package as failed in environments without Docker.
func runTests(m *testing.M) int {
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
			fmt.Fprintf(os.Stderr,
				"repository tests: skipping — could not start postgres container: %v\n", err)
			return 0
		}

		dsn, err = pgc.ConnectionString(ctx, "sslmode=disable")
		if err != nil {
			pgc.Terminate(ctx) //nolint:errcheck
			fmt.Fprintf(os.Stderr, "repository tests: connection string: %v\n", err)
			return 1
		}
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		if pgc != nil {
			pgc.Terminate(ctx) //nolint:errcheck
		}
		fmt.Fprintf(os.Stderr, "repository tests: open pool: %v\n", err)
		return 1
	}

	if err := applyMigrations(ctx, pool); err != nil {
		pool.Close()
		if pgc != nil {
			pgc.Terminate(ctx) //nolint:errcheck
		}
		fmt.Fprintf(os.Stderr, "repository tests: apply migrations: %v\n", err)
		return 1
	}

	testPool = pool
	code := m.Run()

	pool.Close()
	if pgc != nil {
		if err := pgc.Terminate(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "repository tests: terminate container: %v\n", err)
		}
	}
	return code
}

// applyMigrations runs all up-migrations against the pool.
// The path is relative to the package directory (internal/repository/).
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
