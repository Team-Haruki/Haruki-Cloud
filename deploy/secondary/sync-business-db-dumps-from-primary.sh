#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

PRIMARY_SSH_HOST=${HARUKI_PRIMARY_SSH_HOST:-yamamoto.j8.network}
PRIMARY_SSH_PORT=${HARUKI_PRIMARY_SSH_PORT:-60022}
PRIMARY_SSH_USER=${HARUKI_PRIMARY_SSH_USER:-root}
PRIMARY_SSH_KEY=${HARUKI_PRIMARY_SSH_KEY:-/root/.ssh/haruki_primary_sync_ed25519}
PRIMARY_POSTGRES_CONTAINER=${HARUKI_PRIMARY_POSTGRES_CONTAINER:-haruki-postgres}
DUMP_DIR=${HARUKI_BUSINESS_DB_DUMP_DIR:-/data/HarukiServices/data/business-db-dumps}
DB_NAMES=${HARUKI_BUSINESS_DB_DBS:-haruki_bot,haruki_pjsk,haruki_users}
RETENTION=${HARUKI_BUSINESS_DB_DUMP_RETENTION:-72}

require_command() {
  local name="$1"
  if ! command -v "$name" >/dev/null 2>&1; then
    echo "missing required command: $name" >&2
    exit 127
  fi
}

trim() {
  local value="$1"
  value="${value#"${value%%[![:space:]]*}"}"
  value="${value%"${value##*[![:space:]]}"}"
  printf '%s' "$value"
}

validate_identifier() {
  local kind="$1"
  local value="$2"
  if [[ ! "$value" =~ ^[A-Za-z0-9_]+$ ]]; then
    echo "invalid ${kind}: ${value}" >&2
    exit 2
  fi
}

validate_container_name() {
  local value="$1"
  if [[ ! "$value" =~ ^[A-Za-z0-9_.-]+$ ]]; then
    echo "invalid primary postgres container: ${value}" >&2
    exit 2
  fi
}

split_csv() {
  local csv="$1"
  local -n out_ref="$2"
  local raw item
  local -a raw_items
  out_ref=()
  IFS=',' read -r -a raw_items <<< "$csv"
  for raw in "${raw_items[@]}"; do
    item=$(trim "$raw")
    [[ -n "$item" ]] || continue
    out_ref+=("$item")
  done
}

validate_dump() {
  local dump="$1"
  if command -v pg_restore >/dev/null 2>&1; then
    pg_restore -l "$dump" >/dev/null
    return
  fi

  require_command docker
  docker run --rm \
    -v "$(dirname "$dump"):/dumps:ro" \
    postgres:18.3-trixie \
    pg_restore -l "/dumps/$(basename "$dump")" >/dev/null
}

require_command ssh
validate_container_name "$PRIMARY_POSTGRES_CONTAINER"

split_csv "$DB_NAMES" dbs
if (( ${#dbs[@]} == 0 )); then
  echo "HARUKI_BUSINESS_DB_DBS is empty" >&2
  exit 2
fi

mkdir -p "$DUMP_DIR"

for db in "${dbs[@]}"; do
  validate_identifier "database name" "$db"

  stamp=$(date -u +%Y%m%dT%H%M%SZ)
  target="${DUMP_DIR}/${db}.${stamp}.dump"
  tmp="${target}.tmp"

  rm -f "$tmp"
  ssh \
    -i "$PRIMARY_SSH_KEY" \
    -p "$PRIMARY_SSH_PORT" \
    -o BatchMode=yes \
    -o StrictHostKeyChecking=accept-new \
    -o ConnectTimeout=10 \
    "${PRIMARY_SSH_USER}@${PRIMARY_SSH_HOST}" \
    "docker exec ${PRIMARY_POSTGRES_CONTAINER} pg_dump -U postgres -Fc --no-owner --no-acl -d ${db}" > "$tmp"

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
