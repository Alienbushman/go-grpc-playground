# Deployment Guide

The `Dockerfile` uses a multi-stage build: Go compiles a fully static binary in the
builder stage, and the final image is a minimal Alpine container with no Go toolchain.

---

## Build a Docker image

```bash
# Host platform → grpc-server:latest
mage docker:build

# Linux amd64 (most cloud VMs and servers) → grpc-server:amd64
mage docker:buildAmd64

# Linux arm64 (AWS Graviton, Apple Silicon servers) → grpc-server:arm64
mage docker:buildArm64
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

## Export a static Linux binary (no Docker required)

```bash
# amd64 → bin/server-linux-amd64
mage buildLinuxAmd64

# arm64 → bin/server-linux-arm64
mage buildLinuxArm64
```

The resulting binary is statically linked and has no runtime dependencies — copy it to
any Linux host and run it directly.
