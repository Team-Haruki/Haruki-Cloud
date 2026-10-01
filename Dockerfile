# ── Build stage ──────────────────────────────────────────────────────────────
FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS builder

WORKDIR /build
# Modules in their own layer: it only changes with go.mod/go.sum, and CI keeps it in the
# registry build cache.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
# Build args are declared here, right before the build, not at the top: an ARG becomes part of
# the environment of every later RUN, so its per-commit value would re-run `go mod download`.
ARG TARGETOS TARGETARCH
ARG VERSION=dev
# Pure Go like the release binaries: the server's SQLite driver is modernc.org/sqlite
# (mattn/go-sqlite3 is only used by tests). The cache mount speeds up local rebuilds.
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -ldflags="-w -s -X haruki-cloud/version.Version=${VERSION}" -o haruki-server .

# ── Runtime stage ─────────────────────────────────────────────────────────────
FROM alpine:3.24

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
