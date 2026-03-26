# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.26-alpine AS builder

# gcc + musl-dev required for github.com/mattn/go-sqlite3 (cgo)
RUN apk add --no-cache gcc musl-dev

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux \
    go build -ldflags="-w -s" -o haruki-server ./cmd/server

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:latest

RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /build/haruki-server ./

# Config file is expected to be mounted at /app/haruki-db-configs.yaml
# e.g. docker run -v $(pwd)/haruki-db-configs.yaml:/app/haruki-db-configs.yaml ...
VOLUME ["/app/haruki-db-configs.yaml"]

EXPOSE 6666

ENTRYPOINT ["./haruki-server"]
