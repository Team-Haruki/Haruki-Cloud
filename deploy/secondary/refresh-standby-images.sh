#!/usr/bin/env bash
set -Eeuo pipefail

COMPOSE_DIR=${HARUKI_COMPOSE_DIR:-/data/HarukiServices/configs}
ENV_FILE=${HARUKI_ENV_FILE:-${COMPOSE_DIR}/.env}
PROJECT=${HARUKI_COMPOSE_PROJECT:-haruki-production}
FILES_CSV=${HARUKI_COMPOSE_FILES:-docker-compose.yml,docker-compose.production.yml,docker-compose.vm100.yml,docker-compose.vm100.business-db-source.yml}
PROFILES_CSV=${HARUKI_COMPOSE_PROFILES:-standby-cloud,asset-updater}
SERVICES_CSV=${HARUKI_STANDBY_IMAGE_SERVICES:-haruki-cloud,haruki-drawing,haruki-hmes,haruki-deck-service,haruki-sekai-asset-updater}
PULL_POLICY=${HARUKI_STANDBY_IMAGE_PULL_POLICY:-always}
IGNORE_PULL_FAILURES=${HARUKI_STANDBY_IGNORE_PULL_FAILURES:-true}
PRUNE=${HARUKI_STANDBY_IMAGE_PRUNE:-false}

split_csv() {
  local csv="$1"
  local -n out_ref="$2"
  local raw item
  local -a raw_items
  out_ref=()
  IFS=',' read -r -a raw_items <<< "$csv"
  for raw in "${raw_items[@]}"; do
    item="${raw#"${raw%%[![:space:]]*}"}"
    item="${item%"${item##*[![:space:]]}"}"
    [[ -n "$item" ]] || continue
    out_ref+=("$item")
  done
}

split_csv "$FILES_CSV" files
split_csv "$PROFILES_CSV" profiles
split_csv "$SERVICES_CSV" services

if (( ${#services[@]} == 0 )); then
  echo "HARUKI_STANDBY_IMAGE_SERVICES is empty" >&2
  exit 2
fi

compose_args=(-p "$PROJECT")
if [[ -f "$ENV_FILE" ]]; then
  compose_args+=(--env-file "$ENV_FILE")
fi
for profile in "${profiles[@]}"; do
  compose_args+=(--profile "$profile")
done
for file in "${files[@]}"; do
  if [[ -f "${COMPOSE_DIR}/${file}" ]]; then
    compose_args+=(-f "$file")
  fi
done

cd "$COMPOSE_DIR"
pull_args=(pull --policy "$PULL_POLICY")
if [[ "$IGNORE_PULL_FAILURES" == "true" ]]; then
  pull_args+=(--ignore-pull-failures)
fi

docker compose "${compose_args[@]}" "${pull_args[@]}" "${services[@]}"

if [[ "$PRUNE" == "true" ]]; then
  docker image prune -f --filter "until=168h"
fi
