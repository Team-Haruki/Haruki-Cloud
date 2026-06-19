#!/usr/bin/env bash
set -Eeuo pipefail

ASSET_UPDATER_URL=${HARUKI_ASSET_UPDATER_URL:-http://127.0.0.1:12345}
ASSET_CONFIG=${HARUKI_ASSET_CONFIG:-/data/HarukiServices/configs/haruki-asset-configs.yaml}
USER_AGENT=${HARUKI_ASSET_UPDATER_USER_AGENT:-Haruki-Sekai-API/vm100-bootstrap}
POLL_INTERVAL=${HARUKI_ASSET_BOOTSTRAP_POLL_INTERVAL:-30}
REGION_TIMEOUT=${HARUKI_ASSET_BOOTSTRAP_REGION_TIMEOUT:-21600}
REGIONS=${HARUKI_ASSET_BOOTSTRAP_REGIONS:-jp,en,cn,tw,kr}
STATIC_SYNC_SCRIPT=${HARUKI_STATIC_SYNC_SCRIPT:-/data/HarukiServices/scripts/sync-static-assets-from-primary.sh}

declare -A asset_versions=(
  [jp]=6.5.5.30
  [en]=5.3.51.0
  [cn]=6.0.0
  [tw]=6.0.0
  [kr]=6.0.0
)

declare -A asset_hashes=(
  [jp]=cba71fb5-562e-4ead-aff3-5568d75919f2
  [en]=22a5f495-b9c0-4420-987b-f167d1d7e301
  [cn]=
  [tw]=
  [kr]=
)

read_token() {
  awk '
    BEGIN { in_auth = 0 }
    /^  auth:/ { in_auth = 1; next }
    in_auth && /bearer_token:/ {
      sub(/^[^:]*:[[:space:]]*/, "")
      print
      exit
    }
  ' "$ASSET_CONFIG"
}

TOKEN="${HARUKI_ASSET_UPDATER_TOKEN:-$(read_token)}"
if [[ -z "$TOKEN" ]]; then
  echo "asset-updater bearer token is empty" >&2
  exit 2
fi

api() {
  curl -fsS \
    --max-time 30 \
    -A "$USER_AGENT" \
    -H "Authorization: Bearer ${TOKEN}" \
    "$@"
}

active_job_for_region() {
  local region="$1"
  api "${ASSET_UPDATER_URL}/v2/jobs" |
    jq -r --arg region "$region" '
      .jobs[]?
      | select(.region == $region)
      | select(.status == "queued" or .status == "running")
      | .id
    ' |
    head -n 1
}

submit_region() {
  local region="$1"
  local version="${asset_versions[$region]:-}"
  local hash="${asset_hashes[$region]:-}"

  if [[ -z "$version" ]]; then
    echo "missing asset version for region=$region" >&2
    exit 2
  fi

  local payload
  payload=$(jq -cn \
    --arg region "$region" \
    --arg asset_version "$version" \
    --arg asset_hash "$hash" \
    '{region: $region, asset_version: $asset_version, asset_hash: $asset_hash, dry_run: false}')

  api \
    -H "Content-Type: application/json" \
    -d "$payload" \
    "${ASSET_UPDATER_URL}/v2/assets/update" |
    jq -r '.job.id'
}

wait_region() {
  local region="$1"
  local job_id="$2"
  local start now elapsed status message completed total failed
  start=$(date +%s)

  while true; do
    now=$(date +%s)
    elapsed=$((now - start))
    if (( elapsed > REGION_TIMEOUT )); then
      echo "asset job timed out region=$region job=$job_id elapsed=${elapsed}s" >&2
      exit 1
    fi

    local job
    job=$(api "${ASSET_UPDATER_URL}/v2/jobs/${job_id}")
    status=$(jq -r '.job.status // .status' <<<"$job")
    message=$(jq -r '.job.message // .message // ""' <<<"$job")
    completed=$(jq -r '.job.progress.completed_downloads // .progress.completed_downloads // 0' <<<"$job")
    failed=$(jq -r '.job.progress.failed_downloads // .progress.failed_downloads // 0' <<<"$job")
    total=$(jq -r '.job.progress.total_downloads // .progress.total_downloads // 0' <<<"$job")

    echo "region=$region job=$job_id status=$status completed=$completed failed=$failed total=$total message=$message"

    case "$status" in
      completed)
        return 0
        ;;
      failed | cancelled | canceled)
        echo "$job" | jq .
        exit 1
        ;;
    esac

    sleep "$POLL_INTERVAL"
  done
}

IFS=',' read -r -a regions <<<"$REGIONS"
declare -A jobs=()

for raw_region in "${regions[@]}"; do
  region=$(printf '%s' "$raw_region" | xargs)
  [[ -n "$region" ]] || continue

  job_id=$(active_job_for_region "$region")
  if [[ -n "$job_id" ]]; then
    echo "reuse active asset update region=$region job=$job_id"
  else
    echo "starting asset update region=$region"
    job_id=$(submit_region "$region")
    echo "submitted region=$region job=$job_id"
  fi
  jobs[$region]="$job_id"
done

while true; do
  all_done=true
  for raw_region in "${regions[@]}"; do
    region=$(printf '%s' "$raw_region" | xargs)
    [[ -n "$region" ]] || continue
    job_id="${jobs[$region]}"

    job=$(api "${ASSET_UPDATER_URL}/v2/jobs/${job_id}")
    status=$(jq -r '.job.status // .status' <<<"$job")
    message=$(jq -r '.job.message // .message // ""' <<<"$job")
    completed=$(jq -r '.job.progress.completed_downloads // .progress.completed_downloads // 0' <<<"$job")
    failed=$(jq -r '.job.progress.failed_downloads // .progress.failed_downloads // 0' <<<"$job")
    total=$(jq -r '.job.progress.total_downloads // .progress.total_downloads // 0' <<<"$job")

    echo "region=$region job=$job_id status=$status completed=$completed failed=$failed total=$total message=$message"

    case "$status" in
      completed)
        ;;
      failed | cancelled | canceled)
        echo "$job" | jq .
        exit 1
        ;;
      *)
        all_done=false
        ;;
    esac
  done

  if [[ "$all_done" == "true" ]]; then
    break
  fi

  sleep "$POLL_INTERVAL"
done

"$STATIC_SYNC_SCRIPT"

echo "asset bootstrap and extra static sync completed"
