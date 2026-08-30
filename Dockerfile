# ── Build stage ──────────────────────────────────────────────────────────────
FROM golang:1.27-alpine AS builder

# gcc + musl-dev required for github.com/mattn/go-sqlite3 (cgo)
RUN apk add --no-cache gcc musl-dev

ARG VERSION=dev

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=1 GOOS=linux \
    go build -ldflags="-w -s -X haruki-cloud/version.Version=${VERSION}" -o haruki-server .

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:latest

RUN apk add --no-cache ca-certificates postgresql-client tzdata \
    && addgroup -S -g 10001 haruki \
    && adduser -S -D -u 10001 -G haruki -h /home/haruki -s /sbin/nologin haruki \
    && mkdir -p /app /data/haruki \
    && chown -R haruki:haruki /app /data/haruki

WORKDIR /app
COPY --from=builder --chown=root:root --chmod=0555 /build/haruki-server /usr/local/bin/haruki-server

# Config file is expected to be mounted at /app/haruki-cloud.yaml
# e.g. docker run -v $(pwd)/haruki-cloud.yaml:/app/haruki-cloud.yaml ...

EXPOSE 6666

USER haruki:haruki

ENTRYPOINT ["/usr/local/bin/haruki-server"]
