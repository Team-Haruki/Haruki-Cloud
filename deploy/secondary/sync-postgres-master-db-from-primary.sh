#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

PRIMARY_SSH_HOST=${HARUKI_PRIMARY_SSH_HOST:-yamamoto.j8.network}
PRIMARY_SSH_PORT=${HARUKI_PRIMARY_SSH_PORT:-60022}
PRIMARY_SSH_USER=${HARUKI_PRIMARY_SSH_USER:-root}
PRIMARY_SSH_KEY=${HARUKI_PRIMARY_SSH_KEY:-/root/.ssh/haruki_primary_sync_ed25519}
DUMP_DIR=${HARUKI_MASTERDATA_DB_DUMP_DIR:-/data/HarukiServices/data/db-dumps}
DB_NAMES=${HARUKI_SYNC_POSTGRES_DBS:-haruki_sekai}
RETENTION=${HARUKI_MASTERDATA_DB_DUMP_RETENTION:-6}

mkdir -p "$DUMP_DIR"

IFS=',' read -r -a dbs <<< "$DB_NAMES"
for raw_db in "${dbs[@]}"; do
  db=$(printf '%s' "$raw_db" | xargs)
  [[ -n "$db" ]] || continue

  if [[ ! "$db" =~ ^[A-Za-z0-9_]+$ ]]; then
    echo "invalid database name: $db" >&2
    exit 2
  fi

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
    "docker exec haruki-postgres pg_dump -U postgres -Fc --no-owner --no-acl -d ${db}" > "$tmp"

  if [[ ! -s "$tmp" ]]; then
    echo "empty dump for $db" >&2
    rm -f "$tmp"
    exit 1
  fi

  docker run --rm \
    -v "${DUMP_DIR}:/dumps:ro" \
    postgres:18.3-trixie \
    pg_restore -l "/dumps/$(basename "$tmp")" >/dev/null

  mv "$tmp" "$target"
  sha256sum "$target" > "${target}.sha256"
  ln -sfn "$(basename "$target")" "${DUMP_DIR}/${db}.latest.dump"
  ln -sfn "$(basename "$target").sha256" "${DUMP_DIR}/${db}.latest.dump.sha256"

  mapfile -t old_dumps < <(find "$DUMP_DIR" -maxdepth 1 -type f -name "${db}.*.dump" | sort)
  if (( ${#old_dumps[@]} > RETENTION )); then
    delete_count=$(( ${#old_dumps[@]} - RETENTION ))
    for old_dump in "${old_dumps[@]:0:delete_count}"; do
      rm -f "$old_dump" "${old_dump}.sha256"
    done
  fi

  echo "synced $db to $target"
done
