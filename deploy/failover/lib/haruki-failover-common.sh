#!/usr/bin/env bash
# Shared helpers for Haruki failover scripts.

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

load_compose_env() {
  local env_file="${HARUKI_ENV_FILE:-${HARUKI_COMPOSE_DIR:-/data/HarukiServices/configs}/.env}"
  if [[ -f "$env_file" ]]; then
    local line key value
    while IFS= read -r line || [[ -n "$line" ]]; do
      line="${line#"${line%%[![:space:]]*}"}"
      line="${line%"${line##*[![:space:]]}"}"
      [[ -n "$line" && "${line:0:1}" != "#" && "$line" == *=* ]] || continue
      key="${line%%=*}"
      value="${line#*=}"
      key="$(trim "$key")"
      validate_identifier "env key" "$key"
      case "$value" in
        \"*\") value="${value#\"}"; value="${value%\"}" ;;
        \'*\') value="${value#\'}"; value="${value%\'}" ;;
      esac
      export "${key}=${value}"
    done < "$env_file"
  fi
}

build_compose_args() {
  local compose_dir="${HARUKI_COMPOSE_DIR:-/data/HarukiServices/configs}"
  local env_file="${HARUKI_ENV_FILE:-${compose_dir}/.env}"
  local project="${HARUKI_COMPOSE_PROJECT:-haruki-production}"
  local files_csv="${HARUKI_COMPOSE_FILES:-}"
  local profiles_csv="${HARUKI_COMPOSE_PROFILES:-}"
  local files profiles file profile

  if [[ -n "$files_csv" ]]; then
    split_csv "$files_csv" files
  else
    files=(docker-compose.yml docker-compose.production.yml docker-compose.vm100.yml)
  fi

  COMPOSE_DIR="$compose_dir"
  COMPOSE_ARGS=(-p "$project")
  if [[ -f "$env_file" ]]; then
    COMPOSE_ARGS+=(--env-file "$env_file")
  fi

  if [[ -n "$profiles_csv" ]]; then
    split_csv "$profiles_csv" profiles
    for profile in "${profiles[@]}"; do
      COMPOSE_ARGS+=(--profile "$profile")
    done
  fi

  for file in "${files[@]}"; do
    if [[ -f "${compose_dir}/${file}" ]]; then
      COMPOSE_ARGS+=(-f "$file")
    fi
  done
}

compose_run() {
  build_compose_args
  cd "$COMPOSE_DIR"
  docker compose "${COMPOSE_ARGS[@]}" "$@"
}

upsert_env_var() {
  local env_file="$1"
  local key="$2"
  local value="$3"
  local tmp

  validate_identifier "env key" "$key"
  mkdir -p "$(dirname "$env_file")"
  touch "$env_file"
  tmp=$(mktemp "${env_file}.XXXXXX")

  awk -v key="$key" -v value="$value" '
    BEGIN { done = 0 }
    $0 ~ "^[[:space:]]*" key "=" {
      print key "=" value
      done = 1
      next
    }
    { print }
    END {
      if (!done) {
        print key "=" value
      }
    }
  ' "$env_file" > "$tmp"

  chmod --reference="$env_file" "$tmp" 2>/dev/null || chmod 600 "$tmp"
  mv "$tmp" "$env_file"
}

wait_for_postgres() {
  local container="${HARUKI_POSTGRES_CONTAINER:-haruki-postgres}"
  local user="${POSTGRES_USER:-postgres}"
  local timeout="${HARUKI_POSTGRES_WAIT_TIMEOUT:-120}"
  local start now
  start=$(date +%s)

  while true; do
    if docker exec "$container" pg_isready -U "$user" -d postgres >/dev/null 2>&1; then
      return 0
    fi
    now=$(date +%s)
    if (( now - start >= timeout )); then
      echo "postgres is not ready after ${timeout}s" >&2
      docker ps --filter "name=${container}" --format 'table {{.Names}}\t{{.Status}}' >&2 || true
      exit 1
    fi
    sleep 2
  done
}
