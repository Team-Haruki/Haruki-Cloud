#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib/haruki-failover-common.sh
. "${SCRIPT_DIR}/lib/haruki-failover-common.sh"

if [[ "${HARUKI_UNFENCE_CONFIRM:-}" != "RESTORED_FROM_BACKUP" ]]; then
  echo "refusing to unfence without HARUKI_UNFENCE_CONFIRM=RESTORED_FROM_BACKUP" >&2
  exit 2
fi

require_command docker

ENV_FILE=${HARUKI_ENV_FILE:-${HARUKI_COMPOSE_DIR:-/data/HarukiServices/configs}/.env}
POSTGRES_CONTAINER=${HARUKI_POSTGRES_CONTAINER:-haruki-postgres}
REDIS_CONTAINER=${HARUKI_REDIS_CONTAINER:-haruki-redis}
CLOUD_CONTAINER=${HARUKI_CLOUD_CONTAINER:-haruki-cloud}
START_SERVICES=${HARUKI_UNFENCE_SERVICES:-postgres,redis,haruki-cloud}

echo "setting HARUKI_NODE_READ_ONLY=false in ${ENV_FILE}"
upsert_env_var "$ENV_FILE" HARUKI_NODE_READ_ONLY false

for container in "$POSTGRES_CONTAINER" "$REDIS_CONTAINER" "$CLOUD_CONTAINER"; do
  if docker inspect "$container" >/dev/null 2>&1; then
    docker update --restart=unless-stopped "$container" >/dev/null || true
  fi
done

split_csv "$START_SERVICES" services
if (( ${#services[@]} == 0 )); then
  echo "HARUKI_UNFENCE_SERVICES is empty" >&2
  exit 2
fi

compose_run up -d "${services[@]}"

echo "local node unfenced and services started: ${START_SERVICES}"
