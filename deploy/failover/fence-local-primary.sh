#!/usr/bin/env bash
set -Eeuo pipefail

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib/haruki-failover-common.sh
. "${SCRIPT_DIR}/lib/haruki-failover-common.sh"

if [[ "${HARUKI_FENCE_CONFIRM:-}" != "FENCE_PRIMARY" ]]; then
  echo "refusing to fence without HARUKI_FENCE_CONFIRM=FENCE_PRIMARY" >&2
  exit 2
fi

require_command docker

ENV_FILE=${HARUKI_ENV_FILE:-${HARUKI_COMPOSE_DIR:-/data/HarukiServices/configs}/.env}
CLOUD_CONTAINER=${HARUKI_CLOUD_CONTAINER:-haruki-cloud}
POSTGRES_CONTAINER=${HARUKI_POSTGRES_CONTAINER:-haruki-postgres}

echo "setting HARUKI_NODE_READ_ONLY=true in ${ENV_FILE}"
upsert_env_var "$ENV_FILE" HARUKI_NODE_READ_ONLY true

for container in "$CLOUD_CONTAINER" "$POSTGRES_CONTAINER"; do
  if docker inspect "$container" >/dev/null 2>&1; then
    docker update --restart=no "$container" >/dev/null
  fi
done

docker stop "$CLOUD_CONTAINER" >/dev/null 2>&1 || true
docker stop "$POSTGRES_CONTAINER" >/dev/null 2>&1 || true

echo "local primary fenced: cloud/postgres stopped and restart policy disabled"
