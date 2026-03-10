# API Testing Guide

The server exposes two interfaces:
- **gRPC** on `:50051` — use `grpcurl` (reflection is registered)
- **HTTP/JSON** on `:8080` — use `curl` or any HTTP client (via the grpc-gateway)

If you'd like a Swagger/OpenAPI description of the HTTP gateway you can
generate a spec with the built‑in mage target (requires the
`protoc-gen-openapiv2` plugin). Run:

```bash
mage gen          # Go, gRPC, gateway handlers _and_ swagger spec
```

The file will appear in `gen/item/item.swagger.json`. You can serve it from the gateway
to power Swagger UI, Redoc, etc. Or run `grpcui` against the gRPC port for a live
Explorer UI (doesn't require a spec).

---

## gRPC (grpcurl)

### List available services

```bash
grpcurl -plaintext localhost:50051 list
```

### CreateItem

```bash
grpcurl -plaintext \
  -d '{"name": "my first item"}' \
  localhost:50051 item.v1.ItemService/CreateItem
```

```json
{
  "item": {
    "id": "9d69498d-c582-4c22-bbb4-658913dcab9a",
    "name": "my first item",
    "createdAt": "2026-03-10T14:41:20+02:00",
    "updatedAt": "2026-03-10T14:41:20+02:00"
  }
}
```

### GetItem

```bash
grpcurl -plaintext \
  -d '{"id": "9d69498d-c582-4c22-bbb4-658913dcab9a"}' \
  localhost:50051 item.v1.ItemService/GetItem
```

```json
{
  "item": {
    "id": "9d69498d-c582-4c22-bbb4-658913dcab9a",
    "name": "my first item",
    "createdAt": "2026-03-10T14:41:20+02:00",
    "updatedAt": "2026-03-10T14:41:20+02:00"
  }
}
```

### UpdateItem

```bash
grpcurl -plaintext \
  -d '{"id": "9d69498d-c582-4c22-bbb4-658913dcab9a", "name": "renamed item"}' \
  localhost:50051 item.v1.ItemService/UpdateItem
```

```json
{
  "item": {
    "id": "9d69498d-c582-4c22-bbb4-658913dcab9a",
    "name": "renamed item",
    "createdAt": "2026-03-10T14:41:20+02:00",
    "updatedAt": "2026-03-10T15:00:00+02:00"
  }
}
```

### DeleteItem

```bash
grpcurl -plaintext \
  -d '{"id": "9d69498d-c582-4c22-bbb4-658913dcab9a"}' \
  localhost:50051 item.v1.ItemService/DeleteItem
```

```json
{}
```

### Error cases

```bash
# Missing name → InvalidArgument
grpcurl -plaintext -d '{"name": ""}' localhost:50051 item.v1.ItemService/CreateItem
# ERROR: Code: InvalidArgument  Message: name is required

# Unknown ID → NotFound
grpcurl -plaintext \
  -d '{"id": "00000000-0000-0000-0000-000000000000"}' \
  localhost:50051 item.v1.ItemService/GetItem
# ERROR: Code: NotFound  Message: item "00000000-0000-0000-0000-000000000000" not found
```

---

## HTTP/JSON gateway (curl)

The same operations are available over HTTP on `:8080`.

### CreateItem

```bash
curl -s -X POST http://localhost:8080/v1/items \
  -H 'Content-Type: application/json' \
  -d '{"name": "my first item"}'
```

```json
{
  "item": {
    "id": "9d69498d-c582-4c22-bbb4-658913dcab9a",
    "name": "my first item",
    "createdAt": "2026-03-10T12:00:00Z",
    "updatedAt": "2026-03-10T12:00:00Z"
  }
}
```

### GetItem

```bash
curl -s http://localhost:8080/v1/items/9d69498d-c582-4c22-bbb4-658913dcab9a
```

### UpdateItem

```bash
curl -s -X PUT http://localhost:8080/v1/items/9d69498d-c582-4c22-bbb4-658913dcab9a \
  -H 'Content-Type: application/json' \
  -d '{"name": "renamed item"}'
```

### DeleteItem

```bash
curl -s -X DELETE http://localhost:8080/v1/items/9d69498d-c582-4c22-bbb4-658913dcab9a
```

### Error cases

```bash
# Missing name → 400 Bad Request
curl -s -X POST http://localhost:8080/v1/items -H 'Content-Type: application/json' -d '{}'
# {"code": 3, "message": "name is required", "details": []}

# Unknown ID → 404 Not Found
curl -s http://localhost:8080/v1/items/00000000-0000-0000-0000-000000000000
# {"code": 5, "message": "item \"00000000-0000-0000-0000-000000000000\" not found", "details": []}
```
