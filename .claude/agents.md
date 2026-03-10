# Agent Development Guide

This file is for AI agents working in this codebase. It supplements [CLAUDE.md](../CLAUDE.md)
with concrete workflow patterns, decision trees, and examples drawn from the actual code.

---

## Before You Start Any Task

1. Read `CLAUDE.md` in the repo root
2. Identify which resource(s) the task touches
3. Locate the relevant files using the 1:1:1 mapping (proto / server / repository)
4. Read those files before writing anything

---

## Environment

| What              | Value                                                                 |
|-------------------|-----------------------------------------------------------------------|
| Go module         | `github.com/rick/grpc-go-experimentation`                             |
| Database          | Postgres 16 in Docker, `localhost:5433`                               |
| DATABASE_URL      | `postgres://grpc:grpc@localhost:5433/grpc_experiment?sslmode=disable` |
| gRPC port         | `50051` (default)                                                     |
| Build tool        | `mage` — targets in `magefile.go` (`mage -l` to list)                |
| Migrations        | `mage db:migrateUp` / `mage db:migrateDown`                          |
| Mock generation   | `mage mock` or `mockery` directly (config in `.mockery.yaml`)         |

---

## Test Layers

This project has three distinct test layers. Know which one you need before writing a test.

| Layer | File pattern                            | Transport      | DB needed | Mock strategy        |
|-------|-----------------------------------------|----------------|-----------|----------------------|
| 1     | `internal/server/*_test.go`             | None (direct)  | No        | Hand-written structs |
| 2     | `internal/server/*_integration_test.go` | Real gRPC over bufconn | No | Mockery `EXPECT()` |
| 3     | `internal/repository/*_test.go`         | None           | Yes       | Real Postgres        |

---

## Task Patterns

### Pattern: Add a New Resource

**Trigger:** User asks to add a new entity (e.g. "add a `Widget` resource").

**Steps:**

```
1.  proto/widget/widget.proto             → define the service and messages
2.  mage gen                              → regenerate bindings into gen/widget/
3.  migrations/N_create_widgets.up.sql    → create the table
4.  migrations/N_create_widgets.down.sql  → drop the table
5.  docker exec ... < .../N_create_widgets.up.sql → apply migration
6.  internal/repository/widget.go         → WidgetRepository interface (with //go:generate) + pgx impl
7.  .mockery.yaml                         → add WidgetRepository entry
8.  mockery                               → generate mocks/repository/mock_WidgetRepository.go
9.  internal/server/widget.go             → WidgetServer embedding UnimplementedWidgetServiceServer
10. cmd/server/main.go                    → register WidgetServer with grpc.NewServer()
11. grpcurl verify                        → confirm the happy path works before writing tests
12. internal/server/widget_test.go        → Layer 1: hand-written mock, table-driven, direct calls
13. internal/server/widget_integration_test.go → Layer 2: bufconn + mockery mock
14. internal/repository/widget_test.go   → Layer 3: real DB (skip if no DATABASE_URL)
15. README.md                             → update Services table
```

Always complete steps 1–2 before writing any Go code. The generated types are required
by both the handler and the repository implementations.

---

### Pattern: Fix a Bug in a Handler

**Trigger:** User reports incorrect behaviour for a specific RPC.

**Steps:**

1. Read `internal/server/<resource>.go` — find the method
2. Read `internal/repository/<resource>.go` — check the DB query
3. Read `proto/<resource>/<resource>.proto` — verify the contract has not changed
4. Write a failing test that reproduces the bug (prefer Layer 2 — it tests the full stack)
5. Fix the implementation
6. Confirm the test passes, then verify with `grpcurl`

---

### Pattern: Add a Field to an Existing Resource

**Trigger:** User wants to add a field to a proto message (e.g. add `description` to `Item`).

**Steps:**

1. Add the field to the proto message and any relevant request/response types
2. Run `mage gen`
3. Write a migration: `migrations/N_add_description_to_items.up.sql`
4. Apply: `docker exec -i grpc_experiment_db psql -U grpc -d grpc_experiment < migrations/<file>.up.sql`
5. Update `internal/repository/<resource>.go` — add the column to INSERT/UPDATE/SELECT
6. Update `internal/server/<resource>.go` — update the `toProto` mapping function
7. Update affected tests

**Important:** Adding a field is backwards compatible as long as you assign a new field
number. Never reuse field numbers from deleted fields.

---

### Pattern: Add More RPCs to an Existing Service

**Trigger:** User wants to add `UpdateItem`, `DeleteItem`, or `ListItems`.

**Steps:**

1. Add the new RPC + request/response messages to the existing `.proto` file
2. Run `mage gen`
3. Add the method to the `ItemRepository` interface
4. Run `mockery` to regenerate `mocks/repository/mock_ItemRepository.go`
5. Implement the method in `pgxItemRepository`
6. Implement the handler method on `ItemServer` (already embeds `UnimplementedItemServiceServer`
   so the missing method returns `codes.Unimplemented` until you add it)
7. No changes to `main.go` needed
8. Verify with `grpcurl`, then write tests at all three layers

---

## Decision Guide: Where Does This Code Go?

| Question                                          | Answer                              |
|---------------------------------------------------|-------------------------------------|
| Is this a new API contract?                       | Start in `.proto`                   |
| Is this database access?                          | `internal/repository/`              |
| Is this request validation or business logic?     | `internal/server/`                  |
| Is this wiring / startup?                         | `cmd/server/main.go`                |
| Is this a one-off script or tool?                 | Add a target to `magefile.go`      |
| Which test layer does this belong in?             | See Test Layers table above         |

---

## Proto Style Guide

Modelled on the actual `proto/item/item.proto`:

```protobuf
syntax = "proto3";

package item.v1;

// go_package format: "<module>/gen/<resource>;<resource>v1"
// Generated files land at gen/<resource>/ (source-relative, no version subdir)
option go_package = "github.com/Alienbushman/go-grpc-playground/gen/item;itemv1";

message Item {
  string id         = 1;  // UUID as string
  string name       = 2;
  string created_at = 3;  // RFC3339 timestamp as string
  string updated_at = 4;
}

message CreateItemRequest {
  string name = 1;
}

message CreateItemResponse {
  Item item = 1;
}

service ItemService {
  rpc CreateItem(CreateItemRequest) returns (CreateItemResponse);
  rpc GetItem(GetItemRequest)       returns (GetItemResponse);
}
```

Rules:
- `syntax = "proto3"` always
- Package: `<resource>.v1`
- `go_package`: `"<module>/gen/<resource>;<resource>v1"` — no `/v1` subdirectory
- All timestamps: `string` in RFC3339 format
- IDs: `string` (UUIDs generated by Postgres `gen_random_uuid()`)
- Validate in the handler, not in the proto (proto3 has no required fields)

---

## Repository Interface Pattern

Modelled on `internal/repository/item.go`. Note the `//go:generate` directive:

```go
package repository

import (
    "context"
    "time"

    "github.com/jackc/pgx/v5/pgxpool"
)

type Item struct {
    ID        string
    Name      string
    CreatedAt time.Time
    UpdatedAt time.Time
}

// ItemRepository is the interface the server layer depends on.
// Keep it narrow — only add methods that are actually implemented.
//
//go:generate mockery --name=ItemRepository
type ItemRepository interface {
    Create(ctx context.Context, name string) (*Item, error)
    GetByID(ctx context.Context, id string) (*Item, error)
}

type pgxItemRepository struct {
    pool *pgxpool.Pool
}

func NewItemRepository(pool *pgxpool.Pool) ItemRepository {
    return &pgxItemRepository{pool: pool}
}
```

Key points:
- The repository returns raw Go errors — **do not** wrap them in gRPC status errors here
- `pgx.ErrNoRows` is mapped to `codes.NotFound` in the handler, not here
- Use `RETURNING` to avoid a second query after INSERT
- After any interface change, run `mockery` to regenerate the mock

---

## Mockery Config Pattern

`.mockery.yaml` — one entry per interface:

```yaml
with-expecter: true
resolve-type-alias: false
disable-version-string: true
issue-845-fix: true
packages:
  github.com/Alienbushman/go-grpc-playground/internal/repository:
    config:
      dir: mocks/repository
      outpkg: mockrepository
    interfaces:
      ItemRepository:
      WidgetRepository:   # add new interfaces here
```

After editing `.mockery.yaml` or any interface, run `mockery` from the repo root.
Never edit the generated files under `mocks/`.

---

## Layer 1 — Handler Unit Test Pattern

File: `internal/server/item_test.go`, `package server` (same package as the handler).

Uses hand-written mock structs with function fields. Fast, zero setup.

```go
type mockItemRepo struct {
    createFn  func(ctx context.Context, name string) (*repository.Item, error)
    getByIDFn func(ctx context.Context, id string) (*repository.Item, error)
}

func (m *mockItemRepo) Create(ctx context.Context, name string) (*repository.Item, error) {
    return m.createFn(ctx, name)
}

func (m *mockItemRepo) GetByID(ctx context.Context, id string) (*repository.Item, error) {
    return m.getByIDFn(ctx, id)
}

func TestItemServer_GetItem(t *testing.T) {
    tests := []struct {
        name     string
        req      *itemv1.GetItemRequest
        repoFn   func(ctx context.Context, id string) (*repository.Item, error)
        wantCode codes.Code
    }{
        {
            name: "returns item when found",
            req:  &itemv1.GetItemRequest{Id: "abc-123"},
            repoFn: func(_ context.Context, id string) (*repository.Item, error) {
                return &repository.Item{ID: id, Name: "Test"}, nil
            },
            wantCode: codes.OK,
        },
        {
            name:     "returns InvalidArgument when id is empty",
            req:      &itemv1.GetItemRequest{Id: ""},
            wantCode: codes.InvalidArgument,
        },
        {
            name: "returns NotFound when row missing",
            req:  &itemv1.GetItemRequest{Id: "missing"},
            repoFn: func(_ context.Context, _ string) (*repository.Item, error) {
                return nil, pgx.ErrNoRows
            },
            wantCode: codes.NotFound,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            srv := NewItemServer(&mockItemRepo{getByIDFn: tt.repoFn})
            resp, err := srv.GetItem(context.Background(), tt.req)
            assert.Equal(t, tt.wantCode, status.Code(err))
            // ...
        })
    }
}
```

---

## Layer 2 — gRPC Integration Test Pattern

File: `internal/server/item_integration_test.go`, `package server_test` (external package).

Spins up a real gRPC server in-process using `bufconn`. Uses the mockery-generated mock.
Tests the full transport: proto encoding, status translation, response decoding.

```go
package server_test

import (
    "context"
    "net"
    "testing"

    mockrepository "github.com/Alienbushman/go-grpc-playground/mocks/repository"
    "github.com/stretchr/testify/mock"
    "google.golang.org/grpc"
    "google.golang.org/grpc/credentials/insecure"
    "google.golang.org/grpc/test/bufconn"

    itemv1 "github.com/Alienbushman/go-grpc-playground/gen/item"
    "github.com/Alienbushman/go-grpc-playground/internal/repository"
    "github.com/Alienbushman/go-grpc-playground/internal/server"
)

func newTestServer(t *testing.T, repo repository.ItemRepository) itemv1.ItemServiceClient {
    t.Helper()
    lis := bufconn.Listen(1024 * 1024)
    grpcServer := grpc.NewServer()
    itemv1.RegisterItemServiceServer(grpcServer, server.NewItemServer(repo))
    go grpcServer.Serve(lis)
    t.Cleanup(func() { grpcServer.GracefulStop(); lis.Close() })

    conn, _ := grpc.NewClient("passthrough://bufnet",
        grpc.WithContextDialer(func(ctx context.Context, _ string) (net.Conn, error) {
            return lis.DialContext(ctx)
        }),
        grpc.WithTransportCredentials(insecure.NewCredentials()),
    )
    t.Cleanup(func() { conn.Close() })
    return itemv1.NewItemServiceClient(conn)
}

func TestItemService_GetItem_Success(t *testing.T) {
    repo := mockrepository.NewMockItemRepository(t)
    repo.EXPECT().
        GetByID(mock.Anything, "abc-123").   // mock.Anything for context — see note below
        Return(&repository.Item{ID: "abc-123", Name: "test"}, nil)

    client := newTestServer(t, repo)
    resp, err := client.GetItem(context.Background(), &itemv1.GetItemRequest{Id: "abc-123"})

    require.NoError(t, err)
    assert.Equal(t, "abc-123", resp.Item.Id)
}
```

**Critical:** Always use `mock.Anything` for the context argument in `.EXPECT()` calls.
The gRPC server enriches the incoming context with peer info, stream keys, and metadata
before the handler receives it — so `context.Background()` will never match and the
test will hang until timeout.

---

## Layer 3 — DB Integration Test Pattern

File: `internal/repository/item_test.go`, `package repository`.

```go
func connect(t *testing.T) *pgxpool.Pool {
    t.Helper()
    dsn := os.Getenv("DATABASE_URL")
    if dsn == "" {
        t.Skip("DATABASE_URL not set — skipping integration test")
    }
    pool, err := pgxpool.New(context.Background(), dsn)
    require.NoError(t, err)
    require.NoError(t, pool.Ping(context.Background()))
    t.Cleanup(pool.Close)
    return pool
}

func TestItemRepository_GetByID_NotFound(t *testing.T) {
    repo := NewItemRepository(connect(t))

    item, err := repo.GetByID(context.Background(), "00000000-0000-0000-0000-000000000000")

    assert.Nil(t, item)
    assert.True(t, errors.Is(err, pgx.ErrNoRows))
}
```

Always clean up created rows with `t.Cleanup` to keep the DB stable across runs.

---

## End-to-End Verification Commands

Run these after wiring a new resource or RPC. Server must be running.

```bash
# List services (confirms registration + reflection working)
grpcurl -plaintext localhost:50051 list

# CreateItem
grpcurl -plaintext -d '{"name": "test"}' localhost:50051 item.v1.ItemService/CreateItem

# GetItem — paste the id returned above
grpcurl -plaintext -d '{"id": "<uuid>"}' localhost:50051 item.v1.ItemService/GetItem

# Trigger validation error
grpcurl -plaintext -d '{"name": ""}' localhost:50051 item.v1.ItemService/CreateItem
# Expected: Code: InvalidArgument  Message: name is required

# Trigger not-found error
grpcurl -plaintext \
  -d '{"id": "00000000-0000-0000-0000-000000000000"}' \
  localhost:50051 item.v1.ItemService/GetItem
# Expected: Code: NotFound  Message: item "00000000-..." not found
```

---

## Common Mistakes to Avoid

| Mistake                                                    | Correct Approach                                          |
|------------------------------------------------------------|-----------------------------------------------------------|
| Editing files under `gen/`                                 | Edit `.proto`, run `mage gen`                             |
| Editing files under `mocks/`                               | Edit `.mockery.yaml` or the interface, run `mockery`      |
| Using `context.Background()` as a matcher in `.EXPECT()`   | Use `mock.Anything` — gRPC context is always wrapped      |
| Returning raw `error` from a handler                       | Wrap with `status.Errorf(codes.X, ...)`                   |
| Mapping `pgx.ErrNoRows` in the repository                  | Map it in the handler with `errors.Is`                    |
| Accessing DB directly from the server layer                | Always go through the repository interface                |
| Using `log.Println` or `fmt.Println`                       | Use `slog.Info` / `slog.Error`                            |
| Hardcoding config values                                   | Read from environment variables                           |
| Writing one giant test function                            | Use table-driven tests                                    |
| Expanding the interface without running `mockery`          | Run `mockery` after every interface change                |
| Writing only Layer 1 tests for a new resource              | Write all three layers: unit + gRPC integration + DB      |

---

## Asking for Clarification

If a task is ambiguous, ask the user to clarify:

- Which resource is affected?
- Should this be a new RPC or a change to an existing one?
- Is this a breaking change to the API contract?
- Does this require a database migration?

Do not guess on breaking changes. Ask.
