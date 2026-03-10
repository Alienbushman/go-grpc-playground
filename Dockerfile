# ---- Build stage ----
FROM golang:1.26-alpine AS builder

WORKDIR /app

# Cache module downloads separately from source
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO_ENABLED=0 produces a fully static binary — no libc dependency in final image
# -ldflags="-s -w" strips debug info to reduce binary size
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /bin/server ./cmd/server

# ---- Final stage ----
FROM alpine:3.21

# ca-certificates is required for any outbound TLS connections
RUN apk add --no-cache ca-certificates

COPY --from=builder /bin/server /bin/server

EXPOSE 50051
EXPOSE 8080

ENTRYPOINT ["/bin/server"]
