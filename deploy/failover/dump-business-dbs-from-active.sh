#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib/haruki-failover-common.sh
. "${SCRIPT_DIR}/lib/haruki-failover-common.sh"

require_command ssh

ACTIVE_SSH_HOST=${HARUKI_ACTIVE_DATA_SSH_HOST:-yamamoto.j8.network}
ACTIVE_SSH_PORT=${HARUKI_ACTIVE_DATA_SSH_PORT:-60022}
ACTIVE_SSH_USER=${HARUKI_ACTIVE_DATA_SSH_USER:-root}
ACTIVE_SSH_KEY=${HARUKI_ACTIVE_DATA_SSH_KEY:-/root/.ssh/haruki_active_data_ed25519}
ACTIVE_POSTGRES_CONTAINER=${HARUKI_ACTIVE_POSTGRES_CONTAINER:-haruki-postgres}
DUMP_DIR=${HARUKI_BUSINESS_DB_DUMP_DIR:-/data/HarukiServices/data/business-db-dumps}
DB_NAMES=${HARUKI_BUSINESS_DB_DBS:-haruki_bot,haruki_pjsk,haruki_users}
RETENTION=${HARUKI_BUSINESS_DB_DUMP_RETENTION:-36}

if [[ ! "$ACTIVE_POSTGRES_CONTAINER" =~ ^[A-Za-z0-9_.-]+$ ]]; then
  echo "invalid active postgres container: ${ACTIVE_POSTGRES_CONTAINER}" >&2
  exit 2
fi

split_csv "$DB_NAMES" dbs
if (( ${#dbs[@]} == 0 )); then
  echo "HARUKI_BUSINESS_DB_DBS is empty" >&2
  exit 2
fi

mkdir -p "$DUMP_DIR"

validate_dump() {
  local dump="$1"
  if command -v pg_restore >/dev/null 2>&1; then
    pg_restore -l "$dump" >/dev/null
    return
  fi
  if command -v docker >/dev/null 2>&1; then
    docker run --rm \
      -v "$(dirname "$dump"):/dumps:ro" \
      postgres:18.3-trixie \
      pg_restore -l "/dumps/$(basename "$dump")" >/dev/null
    return
  fi
  echo "pg_restore/docker not found; validated dump size only" >&2
}

for db in "${dbs[@]}"; do
  validate_identifier "database name" "$db"

  stamp=$(date -u +%Y%m%dT%H%M%SZ)
  target="${DUMP_DIR}/${db}.${stamp}.dump"
  tmp="${target}.tmp"

  rm -f "$tmp"
  ssh \
    -i "$ACTIVE_SSH_KEY" \
    -p "$ACTIVE_SSH_PORT" \
    -o BatchMode=yes \
    -o StrictHostKeyChecking=accept-new \
    -o ConnectTimeout=10 \
    "${ACTIVE_SSH_USER}@${ACTIVE_SSH_HOST}" \
    "docker exec ${ACTIVE_POSTGRES_CONTAINER} pg_dump -U postgres -Fc --no-owner --no-acl -d ${db}" > "$tmp"

  if [[ ! -s "$tmp" ]]; then
    echo "empty dump for ${db}" >&2
    rm -f "$tmp"
    exit 1
  fi

  validate_dump "$tmp"
  mv "$tmp" "$target"
  (cd "$DUMP_DIR" && sha256sum "$(basename "$target")" > "$(basename "$target").sha256")
  ln -sfn "$(basename "$target")" "${DUMP_DIR}/${db}.latest.dump"
  ln -sfn "$(basename "$target").sha256" "${DUMP_DIR}/${db}.latest.dump.sha256"

  mapfile -t old_dumps < <(find "$DUMP_DIR" -maxdepth 1 -type f -name "${db}.*.dump" | sort)
  if (( ${#old_dumps[@]} > RETENTION )); then
    delete_count=$(( ${#old_dumps[@]} - RETENTION ))
    for old_dump in "${old_dumps[@]:0:delete_count}"; do
      rm -f "$old_dump" "${old_dump}.sha256"
    done
  fi

  echo "synced ${db} to ${target}"
done
