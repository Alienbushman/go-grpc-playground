# gRPC Go CRUD Experimentation

A Go gRPC server demonstrating standard CRUD patterns, structured as an
**agent-development-first** codebase. Every architectural decision is made to maximize
clarity for both humans and AI agents reading and extending the code.

---

## What This Is

This project is a reference implementation and experimentation ground for:

- Idiomatic Go gRPC service structure
- Proto-first API design with HTTP/JSON gateway (gRPC-Gateway v2)
- Clean repository pattern with testable interfaces
- Three-layer test strategy: unit → gRPC integration → DB integration
- Agent-friendly project layout (see [CLAUDE.md](CLAUDE.md))

It is intentionally simple. The goal is correctness and clarity, not scale.

---

## Services

| Service       | Proto file              | RPCs implemented                                    | Description                   |
|---------------|-------------------------|-----------------------------------------------------|-------------------------------|
| `ItemService` | `proto/item/item.proto` | `CreateItem`, `GetItem`, `UpdateItem`, `DeleteItem` | Full CRUD operations on items |

---

## Prerequisites

| Tool                        | Version tested | Install                                                                                     |
|-----------------------------|----------------|---------------------------------------------------------------------------------------------|
| Go                          | 1.26           | https://go.dev/dl/                                                                          |
| `buf`                       | 1.50+          | `go install github.com/bufbuild/buf/cmd/buf@latest`                                         |
| `protoc-gen-go`             | 1.36           | `go install google.golang.org/protobuf/cmd/protoc-gen-go@latest`                            |
| `protoc-gen-go-grpc`        | 1.6            | `go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest`                          |
| `protoc-gen-grpc-gateway`   | 2.28+          | `go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest`      |
| `protoc-gen-openapiv2`      | 2.28+          | `go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest`         |
| `mockery`                   | 2.53+          | `go install github.com/vektra/mockery/v2@latest`                                            |
| `mage`                      | 1.16+          | `go install github.com/magefile/mage@latest`                                                |
| Docker Desktop              | 27+            | https://www.docker.com/products/docker-desktop/ (runs Postgres)                            |
| `grpcurl`                   | latest         | `go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest`                            |

No standalone PostgreSQL installation required — Postgres runs in Docker.

---

## Quick Start

### Returning developer — back in 30 seconds

Already set up? These three commands are all you need:

```bash
mage gen     # regenerate gen/ and mocks/ (gitignored — needed after clone or proto change)
mage dev     # start Postgres, apply migrations, run the server
```

The server is ready when you see:

```
level=INFO msg="gRPC server listening" port=50051
level=INFO msg="HTTP gateway listening" port=8080
```

**Try it immediately with Swagger UI** — the server serves the OpenAPI spec at
`http://localhost:8080/swagger.json`. Open that URL in your browser to verify the spec loaded,
then browse the API interactively in one of these ways:

| Option | How |
|--------|-----|
| Local Swagger UI (Docker) | `docker run --rm -p 8081:8080 -e SWAGGER_JSON_URL=http://host.docker.internal:8080/swagger.json swaggerapi/swagger-ui` then open `http://localhost:8081` |
| Online Swagger Editor | Open [editor.swagger.io](https://editor.swagger.io), click **File → Import URL**, enter `http://localhost:8080/swagger.json` |
| Paste spec | `curl -s http://localhost:8080/swagger.json` → copy output → paste at [editor.swagger.io](https://editor.swagger.io) |

To stop the server press `Ctrl-C`, then `mage down` to stop Postgres.

---

### First-time setup

#### Option A — Full Docker (no Go toolchain required)

Only Docker Desktop is needed.

```bash
# Build and start everything (Postgres + migrations + server)
docker compose --profile full up --build
```

The gRPC server listens on `:50051` and the HTTP/JSON gateway on `:8080`.

To stop and remove containers:

```bash
docker compose --profile full down
```

#### Option B — Local Go + Docker Postgres

Requires Go 1.26+ and Docker Desktop.

```bash
# 1. Install mage (the build tool that runs all other targets)
go install github.com/magefile/mage@v1.16.0

# 2. Install all remaining tools in one command
mage setup

# 3. Verify your environment
mage doctor

# 4. Download Go module dependencies and fetch buf BSR dependencies
go mod download
buf dep update

# 5. Generate proto bindings and mocks (gen/ and mocks/ are not committed)
mage gen

# 6. Start Postgres, apply migrations, and run the server
mage dev
```

The gRPC server listens on `:50051` and the HTTP/JSON gateway on `:8080` by default.

> **Port note:** Docker Compose binds Postgres to `5433` (not `5432`) to avoid conflicts.
> `mage dev` / `mage run` default `DATABASE_URL` to
> `postgres://grpc:grpc@localhost:5433/grpc_experiment?sslmode=disable` when the variable
> is not set. In the full Docker setup (Option A) the server connects to Postgres on `5432`
> internally.

---

## Environment Variables

| Variable       | Required | Default  | Description                         |
|----------------|----------|----------|-------------------------------------|
| `DATABASE_URL` | Yes      | —        | Full PostgreSQL connection string (`mage run` supplies a local default) |
| `GRPC_PORT`    | No       | `50051`  | Port for the gRPC listener          |
| `HTTP_PORT`    | No       | `8080`   | Port for the HTTP/JSON gateway      |

---

## Project Structure

```
.
├── CLAUDE.md                   # AI agent conventions — read before making changes
├── README.md                   # This file
├── API_TESTING.md              # grpcurl and curl examples for all RPCs
├── DEPLOYMENT.md               # Docker image builds and Linux binary export
├── Dockerfile                  # Multi-stage build — static binary on Alpine
├── docker-compose.yml          # Postgres 16 (localhost:5433) + optional full stack
├── magefile.go                 # Go-native build targets (mage up, mage test, etc.)
├── .mockery.yaml               # Mockery config — interfaces to generate mocks for
├── .claude/
│   └── agents.md               # Agent workflow patterns and task guidance
├── proto/                      # Service definitions (source of truth)
│   └── item/
│       └── item.proto
├── buf.yaml                    # buf module config (BSR deps)
├── buf.gen.yaml                # buf code generation config
├── buf.lock                    # buf dependency lock file
├── gen/                        # Generated proto code — DO NOT edit manually (gitignored)
│   └── item/
│       ├── item.pb.go
│       ├── item_grpc.pb.go
│       ├── item.pb.gw.go       # gRPC-Gateway HTTP handlers
│       └── item.swagger.json   # OpenAPI spec (generated by mage gen)
├── mocks/                      # Mockery-generated mocks — DO NOT edit manually (gitignored)
│   └── repository/
│       └── mock_ItemRepository.go
├── cmd/
│   └── server/
│       └── main.go             # Entry point — wires pool → repo → server
├── internal/
│   ├── server/
│   │   ├── item.go                      # gRPC handler for ItemService
│   │   ├── item_test.go                 # Unit tests (no transport, hand-written mocks)
│   │   └── item_integration_test.go     # gRPC tests (bufconn + mockery mocks)
│   └── repository/
│       ├── item.go                      # ItemRepository interface + pgx implementation
│       └── item_test.go                 # DB integration tests (skipped without DATABASE_URL)
├── migrations/
│   ├── 000001_create_items.up.sql
│   └── 000001_create_items.down.sql
```

> **Note on gitignored generated directories:** `gen/` and `mocks/` are not committed.
> Run `mage gen` after cloning to recreate them — it also regenerates mocks automatically.
> `buf.lock` is committed and pins the BSR dependency versions — do not delete it.

---

## Testing the API

See [API_TESTING.md](API_TESTING.md) for full `grpcurl` and `curl` examples covering all
CRUD operations, error cases, and Swagger/OpenAPI generation.

---

## Development

### Common tasks

```bash
mage -l              # list all targets
mage setup           # install all required tools (run once after cloning)
mage doctor          # verify all tools are installed and print versions

mage check           # build + run mock-based tests (no Docker — quick sanity check)

mage dev             # start Postgres + apply migrations + run server (recommended)
mage up              # start Postgres only
mage db:migrateUp    # start Postgres + apply all migrations
mage run             # run the server locally (Postgres must already be up)
mage build           # compile to bin/server

mage gen             # regenerate proto bindings (after editing a .proto file)
mage mock            # regenerate mocks (after changing a repository interface)

mage test            # unit + gRPC integration tests (no DB required)
mage testAll         # all tests including DB integration tests

mage cover           # coverage report: unit + gRPC integration tests → coverage.html
mage coverAll        # coverage report: all tests (requires DATABASE_URL) → coverage.html

mage stack:up        # full stack in Docker (postgres + migrate + server)
mage stack:down      # stop full stack
```

### Deployment (Docker images and Linux binaries)

See [DEPLOYMENT.md](DEPLOYMENT.md) for cross-platform Docker builds, standalone image
usage, and exporting static Linux binaries without Docker.

### Run tests manually

```bash
# Layer 1 — Handler unit tests (no transport, no DB)
go test ./internal/server/... -run TestItemServer

# Layer 2 — gRPC integration tests (real transport via bufconn, no DB)
go test ./internal/server/... -run TestItemService

# Layer 3 — DB integration tests (requires DATABASE_URL)
DATABASE_URL="postgres://grpc:grpc@localhost:5433/grpc_experiment?sslmode=disable" \
  go test ./internal/repository/...

# With race detector (requires CGO_ENABLED=1)
CGO_ENABLED=1 go test -race ./...
```

DB integration tests in `internal/repository/` are skipped automatically when
`DATABASE_URL` is not set.

### Add a new resource

See the [agent task checklist in CLAUDE.md](CLAUDE.md#agent-task-checklist) for the full
step-by-step. The short version:

1. Write the `.proto` file
2. Run `mage gen`
3. Write and apply a migration
4. Implement the repository interface (with `//go:generate mockery --name=...`)
5. Add the interface to `.mockery.yaml` and run `mockery`
6. Implement the gRPC handler
7. Wire it into `main.go`
8. Verify with `grpcurl`
9. Write unit tests, gRPC integration tests, and DB integration tests

---

## gRPC Method Reference

### ItemService (`item.v1.ItemService`)

| RPC          | Request             | Response             | Status        |
|--------------|---------------------|----------------------|---------------|
| `CreateItem` | `CreateItemRequest` | `CreateItemResponse` | Implemented   |
| `GetItem`    | `GetItemRequest`    | `GetItemResponse`    | Implemented   |
| `UpdateItem` | `UpdateItemRequest` | `UpdateItemResponse` | Implemented   |
| `DeleteItem` | `DeleteItemRequest` | `DeleteItemResponse` | Implemented   |
| `ListItems`  | `ListItemsRequest`  | `ListItemsResponse`  | Not yet added |

---

## Test Coverage Summary

| File                                         | Layer | DB needed | Mock strategy         |
|----------------------------------------------|-------|-----------|-----------------------|
| `internal/server/item_test.go`               | 1     | No        | Hand-written structs  |
| `internal/server/item_integration_test.go`   | 2     | No        | Mockery + bufconn     |
| `internal/repository/item_test.go`           | 3     | Yes       | Real Postgres         |

---

## Agent Development Philosophy

**Explicit over implicit.** Every convention is written down in [CLAUDE.md](CLAUDE.md).
Agents should not infer conventions from existing code alone — the written rules take
precedence.

**One resource, one file.** Every resource maps 1:1 to a proto file, server handler file,
repository file, and migration pair. An agent can locate all relevant code for a resource
by searching for its name.

**Interfaces at every seam.** All database access goes through an interface. This means
agents can always write tests for handlers without needing a running database.

**Proto is the contract.** Agents must not change the gRPC API by editing generated code.
All API changes start in the `.proto` file.

**Three test layers.** Unit tests prove handler logic. gRPC integration tests prove the
transport stack. DB integration tests prove the SQL. Each layer catches different bugs.

**Verify before testing.** Use `grpcurl` to confirm the happy path works end-to-end
before writing tests. Catching wiring mistakes early saves time.

**Small, reviewable steps.** Changes should be atomic: one resource, one feature, one
commit. Agents should not batch unrelated changes together.

---

## License

MIT
