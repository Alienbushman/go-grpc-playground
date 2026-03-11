//go:build mage

package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/magefile/mage/mg"
	"github.com/magefile/mage/sh"
)

const (
	defaultDatabaseURL = "postgres://grpc:grpc@localhost:5433/grpc_experiment?sslmode=disable"
	bin                = "bin/server"
	binLinuxAmd64      = "bin/server-linux-amd64"
	binLinuxArm64      = "bin/server-linux-arm64"
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

// Docker groups Docker image build targets.
type Docker mg.Namespace

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
	if err := sh.RunV("buf", "generate"); err != nil {
		return err
	}
	if err := patchSwaggerHost("gen/item/item.swagger.json", "localhost:8080"); err != nil {
		return err
	}
	return Mock()
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

// Build compiles the server binary for the host platform to bin/server.
func Build() error {
	if err := os.MkdirAll("bin", 0o755); err != nil {
		return err
	}
	return sh.RunWithV(map[string]string{"CGO_ENABLED": "0"},
		"go", "build", "-ldflags=-s -w", "-o", bin, "./cmd/server")
}

// BuildLinuxAmd64 cross-compiles a static Linux amd64 binary to bin/server-linux-amd64.
func BuildLinuxAmd64() error {
	if err := os.MkdirAll("bin", 0o755); err != nil {
		return err
	}
	return sh.RunWithV(map[string]string{"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "amd64"},
		"go", "build", "-ldflags=-s -w", "-o", binLinuxAmd64, "./cmd/server")
}

// BuildLinuxArm64 cross-compiles a static Linux arm64 binary to bin/server-linux-arm64.
func BuildLinuxArm64() error {
	if err := os.MkdirAll("bin", 0o755); err != nil {
		return err
	}
	return sh.RunWithV(map[string]string{"CGO_ENABLED": "0", "GOOS": "linux", "GOARCH": "arm64"},
		"go", "build", "-ldflags=-s -w", "-o", binLinuxArm64, "./cmd/server")
}

// Build builds the Docker image for the host platform, tagged grpc-server:latest.
func (Docker) Build() error {
	return sh.RunV("docker", "build", "-t", "grpc-server:latest", ".")
}

// BuildAmd64 builds the Docker image for linux/amd64, tagged grpc-server:amd64.
func (Docker) BuildAmd64() error {
	return sh.RunV("docker", "build", "--platform", "linux/amd64", "-t", "grpc-server:amd64", ".")
}

// BuildArm64 builds the Docker image for linux/arm64, tagged grpc-server:arm64.
func (Docker) BuildArm64() error {
	return sh.RunV("docker", "build", "--platform", "linux/arm64", "-t", "grpc-server:arm64", ".")
}

// Run runs the server locally (requires Postgres to be up).
func Run() error {
	return sh.RunWithV(map[string]string{"DATABASE_URL": databaseURL()},
		"go", "run", "./cmd/server")
}

// Test runs all mock-based tests (handler unit tests + gRPC integration tests).
// No Docker or database required — repository tests are excluded via build tag.
func Test() error {
	return sh.RunV("go", "test", "-count=1", "./...")
}

// TestAll runs all tests including DB integration tests (requires Docker).
// Repository tests spin up a Postgres testcontainer automatically.
// Set DATABASE_URL to use an existing database instead of starting a container.
func TestAll() error {
	return sh.RunV("go", "test", "-count=1", "-tags=integration", "./...")
}

// TestE2E runs end-to-end tests against the real gRPC server and HTTP gateway (requires Docker).
// Spins up a Postgres testcontainer automatically.
// Set DATABASE_URL to use an existing database instead of starting a container.
func TestE2E() error {
	return sh.RunV("go", "test", "-count=1", "-tags=e2e", "-v", "./internal/e2e/...")
}

// Cover runs mock-based tests with coverage and writes coverage.html.
// Output: coverage.out (raw profile) and coverage.html (browser report).
func Cover() error {
	if err := sh.RunV("go", "test", "-count=1",
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

// CoverAll runs all tests including DB integration tests with coverage and writes coverage.html.
// Requires Docker — repository tests spin up a Postgres testcontainer automatically.
func CoverAll() error {
	if err := sh.RunV("go", "test", "-count=1", "-tags=integration",
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

// waitForPostgres polls pg_isready until Postgres is healthy or 60 s elapses.
func waitForPostgres() error {
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		if err := sh.Run("docker", "exec", container,
			"pg_isready", "-U", "grpc", "-d", "grpc_experiment"); err == nil {
			return nil
		}
		time.Sleep(500 * time.Millisecond)
	}
	return fmt.Errorf("postgres did not become ready within 60s")
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
