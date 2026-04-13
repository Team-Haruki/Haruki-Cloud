#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
WORKSPACE_ROOT="$(cd -- "${REPO_ROOT}/.." && pwd)"

REMOTE_HOST="${REMOTE_HOST:-Haruki}"
REMOTE_PORT="${REMOTE_PORT:-}"
REMOTE_DIR="${REMOTE_DIR:-/data/HarukiServices/alpha}"
REMOTE_BINARY_PATH="${REMOTE_BINARY_PATH:-${REMOTE_DIR}/haruki-server}"
REMOTE_LOG_PATH="${REMOTE_LOG_PATH:-${REMOTE_DIR}/logs/haruki-server.out}"

LOCAL_MASTERDATA_DIR="${LOCAL_MASTERDATA_DIR:-${WORKSPACE_ROOT}/deckrec/masterdata}"
LOCAL_BINARY_PATH="${LOCAL_BINARY_PATH:-${REPO_ROOT}/build/haruki-server}"

BUILD_BINARY="${BUILD_BINARY:-1}"
SYNC_MASTERDATA="${SYNC_MASTERDATA:-1}"
LOG_LEVEL="${LOG_LEVEL:-DEBUG}"
GO_BUILD_TARGET="${GO_BUILD_TARGET:-./cmd/server}"

usage() {
	cat <<'EOF'
Usage:
  ./scripts/deploy_alpha_native_deck.sh

Optional environment variables:
  REMOTE_HOST           SSH host alias or user@host (default: Haruki)
  REMOTE_PORT           SSH port for direct host connections
  REMOTE_DIR            Remote working directory (default: /data/HarukiServices/alpha)
  LOCAL_MASTERDATA_DIR  Local deck masterdata root
  LOCAL_BINARY_PATH     Local built binary path
  BUILD_BINARY          1 to build before deploy, 0 to reuse existing binary
  SYNC_MASTERDATA       1 to upload deckrec/masterdata, 0 to skip
  LOG_LEVEL             Remote HARUKI_BACKEND_LOG_LEVEL (default: DEBUG)
  GO_BUILD_TARGET       Go build package target (default: ./cmd/server)
EOF
}

if [[ "${1:-}" == "-h" || "${1:-}" == "--help" ]]; then
	usage
	exit 0
fi

require_cmd() {
	if ! command -v "$1" >/dev/null 2>&1; then
		echo "missing required command: $1" >&2
		exit 1
	fi
}

check_local_paths() {
	if [[ ! -d "${LOCAL_MASTERDATA_DIR}" ]]; then
		echo "masterdata dir not found: ${LOCAL_MASTERDATA_DIR}" >&2
		exit 1
	fi

	for region in jp cn tw kr en; do
		if [[ ! -d "${LOCAL_MASTERDATA_DIR}/${region}" ]]; then
			echo "missing region masterdata dir: ${LOCAL_MASTERDATA_DIR}/${region}" >&2
			exit 1
		fi
	done

	for name in worldBloomSupportDeckBonusesWL1.json worldBloomSupportDeckBonusesWL2.json; do
		if [[ ! -f "${LOCAL_MASTERDATA_DIR}/${name}" ]]; then
			echo "missing static data file: ${LOCAL_MASTERDATA_DIR}/${name}" >&2
			exit 1
		fi
	done
}

ssh_cmd() {
	if [[ -n "${REMOTE_PORT}" ]]; then
		ssh -p "${REMOTE_PORT}" "${REMOTE_HOST}" "$@"
	else
		ssh "${REMOTE_HOST}" "$@"
	fi
}

scp_cmd() {
	if [[ -n "${REMOTE_PORT}" ]]; then
		scp -P "${REMOTE_PORT}" "$@"
	else
		scp "$@"
	fi
}

build_binary() {
	if [[ "${BUILD_BINARY}" != "1" ]]; then
		if [[ ! -f "${LOCAL_BINARY_PATH}" ]]; then
			echo "binary not found and BUILD_BINARY=0: ${LOCAL_BINARY_PATH}" >&2
			exit 1
		fi
		return
	fi

	mkdir -p "$(dirname -- "${LOCAL_BINARY_PATH}")"
	echo "[build] ${LOCAL_BINARY_PATH}"
	(
		cd "${REPO_ROOT}"
		CGO_ENABLED=0 go build -o "${LOCAL_BINARY_PATH}" "${GO_BUILD_TARGET}"
	)
}

prepare_remote() {
	ssh_cmd "mkdir -p '${REMOTE_DIR}' '${REMOTE_DIR}/deckrec' '${REMOTE_DIR}/logs'"
}

upload_binary() {
	echo "[upload] binary"
	scp_cmd "${LOCAL_BINARY_PATH}" "${REMOTE_HOST}:${REMOTE_BINARY_PATH}.new"
}

upload_masterdata() {
	if [[ "${SYNC_MASTERDATA}" != "1" ]]; then
		return
	fi

	echo "[upload] deck masterdata"
	local tmp_archive
	tmp_archive="$(mktemp "${TMPDIR:-/tmp}/haruki-deck-masterdata.XXXXXX.tar")"
	trap 'rm -f -- "${tmp_archive}"' RETURN

	tar -C "${LOCAL_MASTERDATA_DIR}" -cf "${tmp_archive}" .
	scp_cmd "${tmp_archive}" "${REMOTE_HOST}:${REMOTE_DIR}/deckrec/masterdata.tar.tmp"

	ssh_cmd "bash -s" -- "${REMOTE_DIR}" <<'EOF'
set -euo pipefail
remote_dir="$1"
target_dir="${remote_dir}/deckrec/masterdata"
tmp_dir="${remote_dir}/deckrec/masterdata.tmp"
bak_dir="${remote_dir}/deckrec/masterdata.bak"
archive_path="${remote_dir}/deckrec/masterdata.tar.tmp"

rm -rf "${tmp_dir}"
mkdir -p "${tmp_dir}"
tar -C "${tmp_dir}" -xf "${archive_path}"
rm -f "${archive_path}"

rm -rf "${bak_dir}"
if [[ -d "${target_dir}" ]]; then
	mv "${target_dir}" "${bak_dir}"
fi
mv "${tmp_dir}" "${target_dir}"
rm -rf "${bak_dir}"
EOF
}

restart_remote() {
	echo "[restart] haruki-server"
	ssh_cmd "bash -s" -- "${REMOTE_DIR}" "${LOG_LEVEL}" <<'EOF'
set -euo pipefail
remote_dir="$1"
log_level="$2"
binary_path="${remote_dir}/haruki-server"
log_path="${remote_dir}/logs/haruki-server.out"

mv "${binary_path}.new" "${binary_path}"

chmod +x "${binary_path}"

pkill -f "haruki-server" || true

cd "${remote_dir}"
export HARUKI_BACKEND_LOG_LEVEL="${log_level}"
nohup "${binary_path}" > "${log_path}" 2>&1 &
sleep 1
pgrep -af "${binary_path}" || true
echo "[ok] log: ${log_path}"
EOF
}

main() {
	require_cmd ssh
	require_cmd scp
	require_cmd tar
	require_cmd go

	check_local_paths
	build_binary
	prepare_remote
	upload_binary
	upload_masterdata
	restart_remote

	echo
	echo "done"
	echo "remote host: ${REMOTE_HOST}"
	echo "remote dir:  ${REMOTE_DIR}"
	if [[ -n "${REMOTE_PORT}" ]]; then
		echo "log tail:    ssh -p ${REMOTE_PORT} ${REMOTE_HOST} 'tail -f ${REMOTE_LOG_PATH}'"
	else
		echo "log tail:    ssh ${REMOTE_HOST} 'tail -f ${REMOTE_LOG_PATH}'"
	fi
}

main "$@"
