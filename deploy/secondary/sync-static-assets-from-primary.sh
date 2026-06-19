#!/usr/bin/env bash
set -Eeuo pipefail

PRIMARY_SSH_HOST=${HARUKI_PRIMARY_SSH_HOST:-yamamoto.j8.network}
PRIMARY_SSH_PORT=${HARUKI_PRIMARY_SSH_PORT:-60022}
PRIMARY_SSH_USER=${HARUKI_PRIMARY_SSH_USER:-root}
PRIMARY_SSH_KEY=${HARUKI_PRIMARY_SSH_KEY:-/root/.ssh/haruki_primary_sync_ed25519}
PRIMARY_DRAWING_ROOT=${HARUKI_PRIMARY_DRAWING_ROOT:-/data/HarukiServices/data/drawing}
PRIMARY_ASSET_ROOT=${HARUKI_PRIMARY_ASSET_ROOT:-/data/HarukiServices/data/assets}
DRAWING_ROOT=${HARUKI_DRAWING_ROOT:-/data/HarukiServices/data/drawing}
ASSET_ROOT=${HARUKI_ASSET_ROOT:-/data/HarukiServices/data/assets}
RSYNC_BWLIMIT=${HARUKI_STATIC_RSYNC_BWLIMIT:-0}

ssh_cmd="ssh -i ${PRIMARY_SSH_KEY} -p ${PRIMARY_SSH_PORT} -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10"

rsync_base_opts=(
  -a
  --partial
  --human-readable
  --info=stats1,name0
  -e "$ssh_cmd"
)

if [[ "$RSYNC_BWLIMIT" != "0" ]]; then
  rsync_base_opts+=(--bwlimit="$RSYNC_BWLIMIT")
fi

rsync_mirror_opts=("${rsync_base_opts[@]}" --delete)

mkdir -p \
  "$DRAWING_ROOT/static_images" \
  "$DRAWING_ROOT/custom_profile/tmp-font-assets" \
  "$ASSET_ROOT/user_upload/profile_bg"

echo "syncing drawing root fonts"
rsync "${rsync_base_opts[@]}" \
  --include='/*.otf' \
  --include='/*.ttf' \
  --include='/*.ttc' \
  --include='/*.woff' \
  --include='/*.woff2' \
  --exclude='/*' \
  "${PRIMARY_SSH_USER}@${PRIMARY_SSH_HOST}:${PRIMARY_DRAWING_ROOT}/" \
  "$DRAWING_ROOT/"

echo "syncing drawing/custom_profile/tmp-font-assets"
rsync "${rsync_mirror_opts[@]}" \
  "${PRIMARY_SSH_USER}@${PRIMARY_SSH_HOST}:${PRIMARY_DRAWING_ROOT}/custom_profile/tmp-font-assets/" \
  "$DRAWING_ROOT/custom_profile/tmp-font-assets/"

echo "syncing drawing/static_images"
rsync "${rsync_mirror_opts[@]}" \
  "${PRIMARY_SSH_USER}@${PRIMARY_SSH_HOST}:${PRIMARY_DRAWING_ROOT}/static_images/" \
  "$DRAWING_ROOT/static_images/"

echo "syncing asset custom_profile fonts"
for region in jp en cn tw kr; do
  mkdir -p "$ASSET_ROOT/${region}-assets/startapp/custom_profile/font"
  rsync "${rsync_base_opts[@]}" \
    "${PRIMARY_SSH_USER}@${PRIMARY_SSH_HOST}:${PRIMARY_ASSET_ROOT}/${region}-assets/startapp/custom_profile/font/" \
    "$ASSET_ROOT/${region}-assets/startapp/custom_profile/font/"
done

echo "syncing assets/user_upload/profile_bg"
rsync "${rsync_mirror_opts[@]}" \
  "${PRIMARY_SSH_USER}@${PRIMARY_SSH_HOST}:${PRIMARY_ASSET_ROOT}/user_upload/profile_bg/" \
  "$ASSET_ROOT/user_upload/profile_bg/"

echo "static drawing assets and profile backgrounds synced"
