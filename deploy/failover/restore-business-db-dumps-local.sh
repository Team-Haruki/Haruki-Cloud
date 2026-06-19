#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib/haruki-failover-common.sh
. "${SCRIPT_DIR}/lib/haruki-failover-common.sh"

if [[ "${HARUKI_RESTORE_CONFIRM:-}" != "RESTORE_BUSINESS_DBS" ]]; then
  echo "refusing to restore business DBs without HARUKI_RESTORE_CONFIRM=RESTORE_BUSINESS_DBS" >&2
  exit 2
fi

require_command docker

DUMP_DIR=${HARUKI_BUSINESS_DB_DUMP_DIR:-/data/HarukiServices/data/business-db-dumps}
DB_NAMES=${HARUKI_BUSINESS_DB_DBS:-haruki_bot,haruki_pjsk,haruki_users}
POSTGRES_CONTAINER=${HARUKI_POSTGRES_CONTAINER:-haruki-postgres}

load_compose_env
POSTGRES_SUPERUSER=${POSTGRES_USER:-postgres}
split_csv "$DB_NAMES" dbs
if (( ${#dbs[@]} == 0 )); then
  echo "HARUKI_BUSINESS_DB_DBS is empty" >&2
  exit 2
fi

db_owner_for() {
  local db="$1"
  case "$db" in
    "${BOT_DB_NAME:-haruki_bot}")
      printf '%s\n' "${BOT_DB_USER:-haruki_bot}"
      ;;
    "${PJSK_DB_NAME:-haruki_pjsk}")
      printf '%s\n' "${PJSK_DB_USER:-haruki_pjsk}"
      ;;
    "${USERS_DB_NAME:-haruki_users}")
      printf '%s\n' "${USERS_DB_USER:-haruki_users}"
      ;;
    "${CENSOR_DB_NAME:-haruki_censor}")
      printf '%s\n' "${CENSOR_DB_USER:-haruki_censor}"
      ;;
    *)
      printf '%s\n' "${HARUKI_RESTORE_DB_OWNER:-postgres}"
      ;;
  esac
}

verify_dump() {
  local dump="$1"
  local checksum="${dump}.sha256"
  if [[ -s "$checksum" ]]; then
    (cd "$(dirname "$dump")" && sha256sum -c "$(basename "$checksum")")
  fi
}

compose_run up -d postgres
wait_for_postgres

for db in "${dbs[@]}"; do
  validate_identifier "database name" "$db"
  dump_file="${DUMP_DIR}/${db}.latest.dump"
  if [[ ! -s "$dump_file" ]]; then
    echo "dump file not found or empty: ${dump_file}" >&2
    exit 2
  fi
  verify_dump "$dump_file"

  owner=$(db_owner_for "$db")
  validate_identifier "database owner" "$owner"

  echo "restoring ${db} as owner ${owner}"
  docker exec "$POSTGRES_CONTAINER" dropdb -U "$POSTGRES_SUPERUSER" --if-exists --force "$db"
  docker exec "$POSTGRES_CONTAINER" createdb -U "$POSTGRES_SUPERUSER" -O "$owner" "$db"
  docker exec -i "$POSTGRES_CONTAINER" pg_restore \
    -U "$POSTGRES_SUPERUSER" \
    --role "$owner" \
    --no-owner \
    --no-acl \
    --single-transaction \
    --exit-on-error \
    -d "$db" < "$dump_file"
  echo "restored ${db} from ${dump_file}"
done
