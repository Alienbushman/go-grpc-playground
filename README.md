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
| `protoc`                    | 34.0           | https://grpc.io/docs/protoc-installation/                                                   |
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

### Option A — Full Docker (no Go toolchain required)

Only Docker Desktop is needed.

```bash
# 1. Clone
git clone <repo-url>
cd go-grpc-playground

# 2. Build and start everything (Postgres + migrations + server)
docker compose --profile full up --build
```

The gRPC server listens on `:50051` and the HTTP/JSON gateway on `:8080`.

To stop and remove containers:

```bash
docker compose --profile full down
```

---

### Option B — Local Go + Docker Postgres

Requires Go 1.26+, Docker Desktop, and `protoc`. Go-based tools are installed via `go install`.

```bash
# 1. Clone
git clone <repo-url>
cd go-grpc-playground

# 2. Download Go module dependencies
go mod download

# 3. Install protoc (separate binary — not via go install)
#    See https://grpc.io/docs/protoc-installation/ for your OS

# 4. Install Go-based tools (mage, mockery, protoc plugins, grpcurl)
go install github.com/magefile/mage@latest
go install github.com/vektra/mockery/v2@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-grpc-gateway@latest
go install github.com/grpc-ecosystem/grpc-gateway/v2/protoc-gen-openapiv2@latest
go install github.com/fullstorydev/grpcurl/cmd/grpcurl@latest

# 5. Generate proto bindings and mocks (gen/ and mocks/ are not committed)
mage gen
mage mock

# 6. Start Postgres and apply migrations
mage db:migrateUp

# 7. Run the server (DATABASE_URL defaults to the local Docker Postgres)
mage run
```

The gRPC server listens on `:50051` and the HTTP/JSON gateway on `:8080` by default.

> **Port note:** When running locally (Option B), Docker Compose binds Postgres to `5433`
> (not `5432`) to avoid conflicts. `mage run` defaults `DATABASE_URL` to
> `postgres://grpc:grpc@localhost:5433/grpc_experiment?sslmode=disable` when the variable
> is not set. In the full Docker setup (Option A), the server connects to Postgres
> container-internally on port `5432`.

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
├── third_party/                # Vendored proto dependencies (gitignored — see note below)
│   └── googleapis/
│       └── google/api/         # google/api/annotations.proto, http.proto
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

> **Note on gitignored generated directories:** `gen/`, `mocks/`, and `third_party/` are
> not committed. Run `mage gen` and `mage mock` after cloning to recreate them (Option B
> step 4). `third_party/` contains vendored googleapis proto files required by `mage gen`;
> if it is absent, `protoc` will fail — restore it from the
> [googleapis repository](https://github.com/googleapis/googleapis) or re-vendor it.

---

## Testing the API

See [API_TESTING.md](API_TESTING.md) for full `grpcurl` and `curl` examples covering all
CRUD operations, error cases, and Swagger/OpenAPI generation.

---

## Development

### Common tasks

```bash
mage -l              # list all targets

mage up              # start Postgres
mage db:migrateUp    # start Postgres + apply migrations
mage run             # run the server locally
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
