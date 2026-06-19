#!/usr/bin/env bash
set -Eeuo pipefail

COMPOSE_DIR=${HARUKI_COMPOSE_DIR:-/data/HarukiServices/configs}
DUMP_DIR=${HARUKI_MASTERDATA_DB_DUMP_DIR:-/data/HarukiServices/data/db-dumps}
DB_NAME=${1:-${HARUKI_MASTERDATA_DB_NAME:-haruki_sekai}}
DUMP_FILE=${2:-${DUMP_DIR}/${DB_NAME}.latest.dump}

if [[ ! "$DB_NAME" =~ ^[A-Za-z0-9_]+$ ]]; then
  echo "invalid database name: $DB_NAME" >&2
  exit 2
fi

if [[ ! -s "$DUMP_FILE" ]]; then
  echo "dump file not found or empty: $DUMP_FILE" >&2
  exit 2
fi

cd "$COMPOSE_DIR"

docker compose \
  --profile standby-cloud \
  --env-file .env \
  -f docker-compose.yml \
  -f docker-compose.production.yml \
  -f docker-compose.vm100.yml \
  up -d postgres

for _ in $(seq 1 60); do
  status=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' haruki-postgres 2>/dev/null || true)
  if [[ "$status" == "healthy" || "$status" == "running" ]]; then
    break
  fi
  sleep 2
done

status=$(docker inspect -f '{{if .State.Health}}{{.State.Health.Status}}{{else}}{{.State.Status}}{{end}}' haruki-postgres 2>/dev/null || true)
if [[ "$status" != "healthy" && "$status" != "running" ]]; then
  echo "haruki-postgres is not ready: ${status:-missing}" >&2
  exit 1
fi

if ! docker exec haruki-postgres psql -U postgres -Atc "select 1 from pg_database where datname='${DB_NAME}'" | grep -qx 1; then
  docker exec haruki-postgres createdb -U postgres "$DB_NAME"
fi

docker exec -i haruki-postgres pg_restore \
  -U postgres \
  --clean \
  --if-exists \
  --no-owner \
  --no-acl \
  -d "$DB_NAME" < "$DUMP_FILE"

echo "restored $DB_NAME from $DUMP_FILE"
