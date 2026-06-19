#!/usr/bin/env bash
set -Eeuo pipefail

PRIMARY_SSH_HOST=${HARUKI_PRIMARY_SSH_HOST:-yamamoto.j8.network}
PRIMARY_SSH_PORT=${HARUKI_PRIMARY_SSH_PORT:-60022}
PRIMARY_SSH_USER=${HARUKI_PRIMARY_SSH_USER:-root}
PRIMARY_SSH_KEY=${HARUKI_PRIMARY_SSH_KEY:-/root/.ssh/haruki_primary_sync_ed25519}
PRIMARY_DECK_ROOT=${HARUKI_PRIMARY_DECKREC_ROOT:-/data/HarukiServices/data/deckrec-masterdata}
DECK_ROOT=${DECKREC_ROOT:-/data/HarukiServices/data/deckrec-masterdata}
DECK_SERVICE_URL=${DECK_SERVICE_URL:-http://127.0.0.1:3000}

files=(
  worldBloomSupportDeckBonusesWL1.json
  worldBloomSupportDeckBonusesWL2.json
  worldBloomSupportDeckBonusesWL3.json
)

mkdir -p "$DECK_ROOT"

sources=()
for file in "${files[@]}"; do
  sources+=("${PRIMARY_SSH_USER}@${PRIMARY_SSH_HOST}:${PRIMARY_DECK_ROOT}/${file}")
done

changes=$(
  rsync -az --itemize-changes \
    -e "ssh -i ${PRIMARY_SSH_KEY} -p ${PRIMARY_SSH_PORT} -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10" \
    "${sources[@]}" \
    "$DECK_ROOT/"
)

for file in "${files[@]}"; do
  if [[ ! -s "$DECK_ROOT/$file" ]]; then
    echo "missing deckrec supplement: $DECK_ROOT/$file" >&2
    exit 1
  fi
done

if [[ -n "$changes" ]]; then
  printf '%s\n' "$changes"
  for region in jp en cn tw kr; do
    curl -fsS --max-time 30 \
      -H "Content-Type: application/json" \
      -d "{\"base_dir\":\"/masterdata\",\"region\":\"$region\"}" \
      "$DECK_SERVICE_URL/update/masterdata" >/dev/null || true
  done
fi

echo "deckrec supplements synced"
