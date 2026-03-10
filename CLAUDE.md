# CLAUDE.md — gRPC Go CRUD Experimentation

This file provides conventions, architecture decisions, and instructions for AI agents
working in this codebase. Read this before making any changes.

---

## Project Overview

A Go gRPC server implementing standard CRUD operations. The project is designed as an
**agent-development-first** codebase — meaning it is structured to be easy for AI agents
to read, reason about, extend, and test with minimal ambiguity.

---

## Tech Stack

| Concern             | Tool / Library                                                                             |
|---------------------|--------------------------------------------------------------------------------------------|
| Language            | Go 1.26                                                                                    |
| RPC framework       | `google.golang.org/grpc` v1.79+                                                            |
| HTTP/JSON gateway   | `github.com/grpc-ecosystem/grpc-gateway/v2` v2.28+ (`protoc-gen-grpc-gateway`)            |
| Proto compiler      | `protoc` v34 + `protoc-gen-go` v1.36 + `protoc-gen-go-grpc` v1.6                          |
| Database driver     | `github.com/jackc/pgx/v5` (pgxpool)                                                       |
| Database            | PostgreSQL 16 (via Docker)                                                                 |
| Migrations          | Raw SQL applied via `docker exec psql`                                                     |
| Mock generation     | `mockery` v2.53+ (generates typed mocks from interfaces)                                   |
| Testing             | `testing` stdlib + `testify` + `bufconn` (in-process gRPC transport)                      |
| Build tool          | `mage` (magefile.go — Go-native, replaces make)                                            |
| Config              | Environment variables via `os.Getenv`                                                      |

---

## Repository Layout

```
.
├── CLAUDE.md                   # This file — read first
├── README.md                   # Human-facing docs
├── docker-compose.yml          # Postgres 16 on localhost:5433
├── magefile.go                 # Go-native build targets (run: mage -l)
├── .mockery.yaml               # Mockery config — which interfaces to mock
├── .claude/
│   └── agents.md               # Agent workflow and task patterns
├── proto/
│   └── <resource>/
│       └── <resource>.proto    # Source of truth for all service contracts
├── third_party/
│   └── googleapis/
│       └── google/api/         # Vendored google/api/annotations.proto + http.proto
├── gen/
│   └── <resource>/             # Generated Go code from proto (DO NOT edit manually)
├── mocks/
│   └── <package>/              # Mockery-generated mocks (DO NOT edit manually)
│       └── mock_<Interface>.go
├── cmd/
│   └── server/
│       └── main.go             # Entry point — wires dependencies, starts server
├── internal/
│   ├── server/                 # gRPC handler implementations (one file per resource)
│   └── repository/             # Database access layer (one file per resource)
├── migrations/                 # SQL migration files (up/down pairs)
├── go.mod
└── go.sum
```

---

## Core Conventions

### Proto First

The `.proto` file is the **source of truth** for every service contract. All changes to
an API must start in the proto file, followed by regeneration, then implementation.

**Never edit files under `gen/` directly** — they are always overwritten by `mage gen`.

### gRPC-Gateway HTTP Routing

Every RPC that should be reachable over HTTP/JSON must have a `google.api.http` annotation
in the proto file. The pattern for a CRUD resource is:

```protobuf
import "google/api/annotations.proto";

service ItemService {
  rpc CreateItem(CreateItemRequest) returns (CreateItemResponse) {
    option (google.api.http) = { post: "/v1/items" body: "*" };
  }
  rpc GetItem(GetItemRequest) returns (GetItemResponse) {
    option (google.api.http) = { get: "/v1/items/{id}" };
  }
  rpc UpdateItem(UpdateItemRequest) returns (UpdateItemResponse) {
    option (google.api.http) = { put: "/v1/items/{id}" body: "*" };
  }
  rpc DeleteItem(DeleteItemRequest) returns (DeleteItemResponse) {
    option (google.api.http) = { delete: "/v1/items/{id}" };
  }
}
```

The googleapis proto files (`google/api/annotations.proto`, `google/api/http.proto`) are
vendored into `third_party/googleapis/` — do not delete them.

`mage gen` passes `--proto_path=third_party/googleapis` so protoc can resolve the import.
The gateway is registered in `main.go` and listens on `HTTP_PORT` (default `8080`).

### Module and Package Naming

- Go module: `github.com/rick/grpc-go-experimentation`
- Proto package: `<resource>.v1`
- `go_package` option format: `github.com/Alienbushman/go-grpc-playground/gen/<resource>;<resource>v1`
- Generated files land at: `gen/<resource>/` (source-relative output, not versioned subdirs)

Example for the `item` resource:
```protobuf
option go_package = "github.com/Alienbushman/go-grpc-playground/gen/item;itemv1";
```
This produces `gen/item/item.pb.go` with `package itemv1`.

### One Resource Per File

Each gRPC resource (e.g. `Item`, `User`) has exactly:
- One proto file: `proto/<resource>/<resource>.proto`
- One server handler file: `internal/server/<resource>.go`
- One repository file: `internal/repository/<resource>.go`
- One migration pair: `migrations/<N>_create_<resource>s.up.sql` / `.down.sql`

This 1:1:1 mapping makes it trivial for an agent to locate all code related to a resource.

### CRUD Method Naming

All services follow the same RPC naming pattern:

```protobuf
service ItemService {
  rpc CreateItem(CreateItemRequest)   returns (CreateItemResponse);
  rpc GetItem(GetItemRequest)         returns (GetItemResponse);
  rpc UpdateItem(UpdateItemRequest)   returns (UpdateItemResponse);
  rpc DeleteItem(DeleteItemRequest)   returns (DeleteItemResponse);
  rpc ListItems(ListItemsRequest)     returns (ListItemsResponse);
}
```

Request/response types are always named `<Verb><Resource>Request` / `<Verb><Resource>Response`.

**Currently implemented:** `CreateItem`, `GetItem`, `UpdateItem`, `DeleteItem`. `ListItems` is not yet added.

### Error Handling

Use gRPC status codes. Do not return raw Go errors across the RPC boundary.

```go
import "google.golang.org/grpc/codes"
import "google.golang.org/grpc/status"

return nil, status.Errorf(codes.NotFound, "item %q not found", id)
```

Map `pgx.ErrNoRows` to `codes.NotFound` in the handler — not in the repository:

```go
if errors.Is(err, pgx.ErrNoRows) {
    return nil, status.Errorf(codes.NotFound, "item %q not found", req.Id)
}
```

Standard mappings:
| Scenario                  | gRPC Code               |
|---------------------------|-------------------------|
| Record not found          | `codes.NotFound`        |
| Invalid input             | `codes.InvalidArgument` |
| Already exists            | `codes.AlreadyExists`   |
| Unauthorized              | `codes.Unauthenticated` |
| Internal / unexpected     | `codes.Internal`        |

### Logging

Use `log/slog` (stdlib). Structured key-value pairs only. No `fmt.Println` in production paths.

```go
slog.Info("item created", "id", item.ID, "name", item.Name)
slog.Error("failed to query db", "error", err)
```

### Configuration

All config comes from environment variables. No config files. No hardcoded values.

```go
dsn := os.Getenv("DATABASE_URL")   // required — fail fast if missing
port := os.Getenv("GRPC_PORT")     // optional, default "50051"
port := os.Getenv("HTTP_PORT")     // optional, default "8080"
```

Fail fast at startup if required env vars are missing.

### gRPC Reflection

The server registers `reflection.Register(grpcServer)` so that `grpcurl` can be used
without passing a proto file or descriptor. This must be kept in `main.go`.

---

## Development Commands

Build targets are defined in `magefile.go` using [Mage](https://magefile.org).
Run `mage -l` to list all targets.

```bash
# List all targets
mage -l

# Start Postgres (binds to localhost:5433 — 5432 may already be in use)
mage up

# Stop containers (keeps volumes)
mage down

# Start full stack in Docker (postgres + migrate + server)
mage stack:up

# Stop full stack
mage stack:down

# Apply migrations (starts Postgres first, waits for health)
mage db:migrateUp

# Roll back last migration
mage db:migrateDown

# Regenerate proto bindings after editing a .proto file
mage gen

# Regenerate mocks after changing a repository interface
mage mock

# Build the server binary
mage build

# Run the server locally
mage run

# Run unit + gRPC integration tests (no DB required)
mage test

# Run all tests including DB integration tests
mage testAll

# Coverage report (no DB) — prints per-function % and writes coverage.html
mage cover

# Coverage report (all tests, requires DATABASE_URL)
mage coverAll

# List all available RPCs (uses gRPC reflection — server must be running)
grpcurl -plaintext localhost:50051 list

# CreateItem
grpcurl -plaintext -d '{"name": "my item"}' localhost:50051 item.v1.ItemService/CreateItem

# GetItem
grpcurl -plaintext -d '{"id": "<uuid>"}' localhost:50051 item.v1.ItemService/GetItem

# Run tests with race detector (requires CGO_ENABLED=1)
CGO_ENABLED=1 go test -race ./...
```

---

## Testing Conventions

There are three layers of tests in this project:

### Layer 1 — Handler unit tests (`internal/server/item_test.go`, `package server`)

Call handler methods directly without any gRPC transport. Use hand-written mocks
(structs with function fields). Fast, no setup required.

```go
type mockItemRepo struct {
    getByIDFn func(ctx context.Context, id string) (*repository.Item, error)
}
srv := NewItemServer(&mockItemRepo{...})
resp, err := srv.GetItem(context.Background(), req)
```

### Layer 2 — gRPC integration tests (`internal/server/item_integration_test.go`, `package server_test`)

Spin up a real in-process gRPC server over `bufconn`, connect a real client, and make
actual RPC calls. Use the mockery-generated `MockItemRepository`. These tests prove the
full transport stack: serialisation, status code translation, and response deserialisation.

```go
repo := mockrepository.NewMockItemRepository(t)
repo.EXPECT().Create(mock.Anything, "name").Return(item, nil)
client := newTestServer(t, repo)  // real gRPC over bufconn
resp, err := client.CreateItem(ctx, req)
```

**Important:** Always use `mock.Anything` for the context parameter in `.EXPECT()` calls.
The gRPC server wraps the incoming context with peer metadata and stream keys before
passing it to the handler, so `context.Background()` will never match.

### Layer 3 — DB integration tests (`internal/repository/item_test.go`, `package repository`)

Require a real PostgreSQL connection. Skipped automatically when `DATABASE_URL` is unset:

```go
if os.Getenv("DATABASE_URL") == "" {
    t.Skip("DATABASE_URL not set")
}
```

Use `t.Cleanup` to delete rows created during tests so the DB stays clean.

---

## Mock Generation with Mockery

Mocks are configured in `.mockery.yaml` and generated into `mocks/<package>/`.
**Never edit files under `mocks/` directly** — they are overwritten by `mockery`.

Each repository interface carries a `go:generate` directive:

```go
//go:generate mockery --name=ItemRepository
type ItemRepository interface { ... }
```

Regenerate after any interface change:

```bash
mockery
```

The generated mock uses the typed `EXPECT()` API and auto-asserts all expectations at
test cleanup when constructed with `NewMockItemRepository(t)`.

---

## Agent Task Checklist

When adding a new resource (e.g. `Widget`), an agent should complete these steps in order:

1. [ ] Define `proto/widget/widget.proto` — start with only the RPCs needed, not all 5; add `google.api.http` annotations for any RPCs that need HTTP access
2. [ ] Run `mage gen` to generate bindings in `gen/widget/` (produces `.pb.go`, `_grpc.pb.go`, `.pb.gw.go`)
3. [ ] Write `migrations/<N>_create_widgets.up.sql` and `.down.sql`
4. [ ] Apply the migration: `docker exec -i grpc_experiment_db psql -U grpc -d grpc_experiment < migrations/<file>.up.sql`
5. [ ] Implement `internal/repository/widget.go` — interface with `//go:generate mockery --name=WidgetRepository` + `pgxWidgetRepository` struct
6. [ ] Add the new interface to `.mockery.yaml` and run `mockery` to generate `mocks/repository/mock_WidgetRepository.go`
7. [ ] Implement `internal/server/widget.go` — embed `UnimplementedWidgetServiceServer`, depend on the interface
8. [ ] Wire the new server into `cmd/server/main.go` — register with `grpcServer` and call `RegisterWidgetServiceHandlerFromEndpoint` on the gateway mux
9. [ ] Verify with `grpcurl` (gRPC) and `curl` (HTTP) before writing tests
10. [ ] Write unit tests in `internal/server/widget_test.go` with a hand-written mock
11. [ ] Write gRPC integration tests in `internal/server/widget_integration_test.go` using `bufconn` + mockery mock
12. [ ] Write DB integration tests in `internal/repository/widget_test.go` (skip if no `DATABASE_URL`)
13. [ ] Update the Services table in `README.md`

---

## What Agents Should NOT Do

- Do not edit files under `gen/` — run `mage gen` instead
- Do not edit files under `mocks/` — run `mockery` instead
- Do not delete files under `third_party/googleapis/` — they are required by `mage gen`
- Do not add `init()` functions
- Do not use global mutable state
- Do not use `panic` in request handlers
- Do not return raw `error` values across the gRPC boundary — always wrap with `status.Errorf`
- Do not map `pgx.ErrNoRows` in the repository layer — do it in the handler
- Do not use `context.Background()` as a matcher in mockery `.EXPECT()` calls — use `mock.Anything`
- Do not add new dependencies without noting them in this file and `go.mod`
- Do not skip the repository interface layer — all DB access goes through an interface
  so it can be mocked in unit tests
