//go:build mage

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const (
	defaultDatabaseURL = "postgres://grpc:grpc@localhost:5433/grpc_experiment?sslmode=disable"
	bin                = "bin/server"
	container          = "grpc_experiment_db"
	migrationUp        = "migrations/000001_create_items.up.sql"
	migrationDown      = "migrations/000001_create_items.down.sql"
)

func databaseURL() string {
	if v := os.Getenv("DATABASE_URL"); v != "" {
		return v
	}
	return defaultDatabaseURL
}

// DB groups database-related targets.
type DB mg.Namespace

// Stack groups full-stack docker compose targets.
type Stack mg.Namespace

// Up starts the Postgres container.
func Up() error {
	return sh.RunV("docker", "compose", "up", "-d", "postgres")
}

// Down stops and removes containers (keeps volumes).
func Down() error {
	return sh.RunV("docker", "compose", "down")
}

// Up starts the full stack in Docker (postgres + migrate + server).
func (Stack) Up() error {
	return sh.RunV("docker", "compose", "--profile", "full", "up", "-d", "--build")
}

// Down stops the full stack.
func (Stack) Down() error {
	return sh.RunV("docker", "compose", "--profile", "full", "down")
}

// MigrateUp applies all migrations (starts Postgres first, waits for health).
func (DB) MigrateUp() error {
	mg.Deps(Up)
	fmt.Println("Waiting for Postgres...")
	if err := waitForPostgres(); err != nil {
		return err
	}
	return runSQL(migrationUp)
}

// MigrateDown rolls back the last migration.
func (DB) MigrateDown() error {
	return runSQL(migrationDown)
}

// Gen regenerates proto bindings from all proto files under proto/.
func Gen() error {
	gopath, err := sh.Output("go", "env", "GOPATH")
	if err != nil {
		return fmt.Errorf("go env GOPATH: %w", err)
	}
	env := map[string]string{
		"PATH": os.Getenv("PATH") + string(os.PathListSeparator) + filepath.Join(gopath, "bin"),
	}
	if err := sh.RunWithV(env, "protoc",
		"--proto_path=proto",
		"--proto_path=third_party/googleapis",
		"--go_out=gen",
		"--go_opt=paths=source_relative",
		"--go-grpc_out=gen",
		"--go-grpc_opt=paths=source_relative",
		"--grpc-gateway_out=gen",
		"--grpc-gateway_opt=paths=source_relative",
		"--openapiv2_out=gen",
		"--openapiv2_opt=logtostderr=true",
		"proto/item/item.proto",
	); err != nil {
		return err
	}

	return patchSwaggerHost("gen/item/item.swagger.json", "localhost:8080")
}

func patchSwaggerHost(swaggerFile, host string) error {
	data, err := os.ReadFile(swaggerFile)
	if err != nil {
		return fmt.Errorf("read %s: %w", swaggerFile, err)
	}

	var spec map[string]any
	if err := json.Unmarshal(data, &spec); err != nil {
		return fmt.Errorf("parse %s: %w", swaggerFile, err)
	}

	spec["host"] = host

	out, err := json.MarshalIndent(spec, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal %s: %w", swaggerFile, err)
	}

	if err := os.WriteFile(swaggerFile, out, 0o644); err != nil {
		return fmt.Errorf("write %s: %w", swaggerFile, err)
	}

	fmt.Printf("patched swagger host → %s\n", host)
	return nil
}

// Swagger generates a swagger/OpenAPI specification file from the proto
// definitions. The output will be written into the gen/ directory alongside
// the other generated code. This target is also invoked automatically by
// `mage gen`.
func Swagger() error {
	return Gen()
}

// Mock regenerates mockery mocks.
func Mock() error {
	return sh.RunV("mockery")
}

// Build compiles the server binary to bin/server.
func Build() error {
	if err := os.MkdirAll("bin", 0o755); err != nil {
		return err
	}
	return sh.RunWithV(map[string]string{"CGO_ENABLED": "0"},
		"go", "build", "-ldflags=-s -w", "-o", bin, "./cmd/server")
}

// Run runs the server locally (requires Postgres to be up).
func Run() error {
	return sh.RunWithV(map[string]string{"DATABASE_URL": databaseURL()},
		"go", "run", "./cmd/server")
}

// Test runs unit and gRPC integration tests (no DB required).
func Test() error {
	return sh.RunV("go", "test", "-count=1", "./internal/server/...")
}

// TestAll runs all tests including DB integration tests.
func TestAll() error {
	return sh.RunWithV(map[string]string{"DATABASE_URL": databaseURL()},
		"go", "test", "-count=1", "./...")
}

// Cover runs unit + gRPC integration tests with coverage and opens the HTML report.
// Output: coverage.out (raw profile) and coverage.html (browser report).
func Cover() error {
	if err := sh.RunV("go", "test", "-count=1",
		"-coverprofile=coverage.out", "-covermode=atomic",
		"./internal/server/...",
	); err != nil {
		return err
	}
	if err := sh.RunV("go", "tool", "cover", "-func=coverage.out"); err != nil {
		return err
	}
	return sh.RunV("go", "tool", "cover", "-html=coverage.out", "-o=coverage.html")
}

// CoverAll runs all tests (including DB) with coverage and opens the HTML report.
// Requires DATABASE_URL to be set (or uses the default local value).
func CoverAll() error {
	if err := sh.RunWithV(map[string]string{"DATABASE_URL": databaseURL()},
		"go", "test", "-count=1",
		"-coverprofile=coverage.out", "-covermode=atomic",
		"./...",
	); err != nil {
		return err
	}
	if err := sh.RunV("go", "tool", "cover", "-func=coverage.out"); err != nil {
		return err
	}
	return sh.RunV("go", "tool", "cover", "-html=coverage.out", "-o=coverage.html")
}

// waitForPostgres polls pg_isready until Postgres is healthy.
func waitForPostgres() error {
	for {
		err := sh.Run("docker", "exec", container,
			"pg_isready", "-U", "grpc", "-d", "grpc_experiment")
		if err == nil {
			return nil
		}
	}
}

// runSQL pipes a SQL file into psql running inside the DB container.
func runSQL(file string) error {
	f, err := os.Open(file)
	if err != nil {
		return fmt.Errorf("open %s: %w", file, err)
	}
	defer f.Close()

	cmd := exec.Command("docker", "exec", "-i", container,
		"psql", "-U", "grpc", "-d", "grpc_experiment")
	cmd.Stdin = f
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}
