# Deployment Guide

The `Dockerfile` uses a multi-stage build: Go compiles a fully static binary in the
builder stage, and the final image is a minimal Alpine container with no Go toolchain.

---

## Build for the host platform

```bash
docker build -t grpc-server:latest .
```

---

## Build for a specific Linux architecture

Use `--platform` to cross-compile for a target that differs from your machine.
Common targets:

```bash
# Linux x86-64 (most cloud VMs and servers)
docker build --platform linux/amd64 -t grpc-server:amd64 .

# Linux ARM64 (AWS Graviton, Apple Silicon servers)
docker build --platform linux/arm64 -t grpc-server:arm64 .
```

> Docker BuildKit is required for cross-platform builds. It is enabled by default in
> Docker Desktop 4.0+. If you see an error about the builder, run:
> `docker buildx create --use`

---

## Run the image standalone (bring your own Postgres)

```bash
docker run --rm \
  -e DATABASE_URL="postgres://grpc:grpc@<host>:5432/grpc_experiment?sslmode=disable" \
  -p 50051:50051 \
  -p 8080:8080 \
  grpc-server:latest
```

---

## Export the binary without Docker

To produce a Linux binary directly from Go (no Docker required):

```bash
# amd64
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 \
  go build -ldflags="-s -w" -o bin/server-linux-amd64 ./cmd/server

# arm64
GOOS=linux GOARCH=arm64 CGO_ENABLED=0 \
  go build -ldflags="-s -w" -o bin/server-linux-arm64 ./cmd/server
```

The resulting binary is statically linked and has no runtime dependencies — copy it to
any Linux host and run it directly.
