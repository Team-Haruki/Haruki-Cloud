#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

SCRIPT_DIR=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
# shellcheck source=lib/haruki-failover-common.sh
. "${SCRIPT_DIR}/lib/haruki-failover-common.sh"

require_command rsync
require_command ssh

BACKUP_SSH_HOST=${HARUKI_BACKUP_SSH_HOST:-100.80.207.86}
BACKUP_SSH_PORT=${HARUKI_BACKUP_SSH_PORT:-22}
BACKUP_SSH_USER=${HARUKI_BACKUP_SSH_USER:-root}
BACKUP_SSH_KEY=${HARUKI_BACKUP_SSH_KEY:-/root/.ssh/haruki_backup_sync_ed25519}
BACKUP_DUMP_DIR=${HARUKI_BACKUP_BUSINESS_DB_DUMP_DIR:-/data/HarukiServices/data/business-db-dumps}
LOCAL_DUMP_DIR=${HARUKI_BUSINESS_DB_DUMP_DIR:-/data/HarukiServices/data/business-db-dumps}
RSYNC_BWLIMIT=${HARUKI_BUSINESS_DB_RSYNC_BWLIMIT:-0}

mkdir -p "$LOCAL_DUMP_DIR"

rsync_opts=(
  -a
  --delete
  --partial
  --human-readable
  --info=stats1,name0
  -e "ssh -i ${BACKUP_SSH_KEY} -p ${BACKUP_SSH_PORT} -o BatchMode=yes -o StrictHostKeyChecking=accept-new -o ConnectTimeout=10"
)

if [[ "$RSYNC_BWLIMIT" != "0" ]]; then
  rsync_opts+=(--bwlimit="$RSYNC_BWLIMIT")
fi

rsync "${rsync_opts[@]}" \
  "${BACKUP_SSH_USER}@${BACKUP_SSH_HOST}:${BACKUP_DUMP_DIR}/" \
  "$LOCAL_DUMP_DIR/"

echo "business DB dumps synced from backup to ${LOCAL_DUMP_DIR}"
