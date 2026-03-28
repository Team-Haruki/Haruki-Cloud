#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REPO_ROOT="$(cd -- "${SCRIPT_DIR}/.." && pwd)"
WORKSPACE_ROOT="$(cd -- "${REPO_ROOT}/.." && pwd)"

REMOTE_HOST="${REMOTE_HOST:-Haruki}"
REMOTE_DIR="${REMOTE_DIR:-/data/HarukiServices/alpha}"
REMOTE_CONFIG="${REMOTE_CONFIG:-${REMOTE_DIR}/haruki-db-configs.yaml}"
REMOTE_BINARY_PATH="${REMOTE_BINARY_PATH:-${REMOTE_DIR}/haruki-server}"
REMOTE_LIB_PATH="${REMOTE_LIB_PATH:-${REMOTE_DIR}/libsekai_deck_recommend_c.so}"
REMOTE_LOG_PATH="${REMOTE_LOG_PATH:-${REMOTE_DIR}/logs/haruki-server.out}"

LOCAL_MASTERDATA_DIR="${LOCAL_MASTERDATA_DIR:-${WORKSPACE_ROOT}/deckrec/masterdata}"
LOCAL_LIB_PATH="${LOCAL_LIB_PATH:-${REPO_ROOT}/internal/pjsk/render/deck/deck_cgo/lib/linux_amd64/libsekai_deck_recommend_c.so}"
LOCAL_BINARY_PATH="${LOCAL_BINARY_PATH:-${REPO_ROOT}/build/haruki-server}"

BUILD_BINARY="${BUILD_BINARY:-1}"
SYNC_MASTERDATA="${SYNC_MASTERDATA:-1}"
LOG_LEVEL="${LOG_LEVEL:-DEBUG}"
GO_BUILD_TAGS="${GO_BUILD_TAGS:-pjsk_deck_cgo}"
GO_BUILD_TARGET="${GO_BUILD_TARGET:-./cmd/server}"

usage() {
	cat <<'EOF'
Usage:
  ./scripts/deploy_alpha_native_deck.sh

Optional environment variables:
  REMOTE_HOST           SSH host alias or user@host (default: Haruki)
  REMOTE_DIR            Remote working directory (default: /data/HarukiServices/alpha)
  REMOTE_CONFIG         Remote config path to sanity-check
  LOCAL_MASTERDATA_DIR  Local deck masterdata root
  LOCAL_LIB_PATH        Local libsekai_deck_recommend_c.so path
  LOCAL_BINARY_PATH     Local built binary path
  BUILD_BINARY          1 to build before deploy, 0 to reuse existing binary
  SYNC_MASTERDATA       1 to upload deckrec/masterdata, 0 to skip
  LOG_LEVEL             Remote HARUKI_BACKEND_LOG_LEVEL (default: DEBUG)
  GO_BUILD_TAGS         Go build tags (default: pjsk_deck_cgo)
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

	if [[ ! -f "${LOCAL_LIB_PATH}" ]]; then
		echo "deck shared library not found: ${LOCAL_LIB_PATH}" >&2
		exit 1
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
		CGO_ENABLED=1 go build -tags "${GO_BUILD_TAGS}" -o "${LOCAL_BINARY_PATH}" "${GO_BUILD_TARGET}"
	)
}

check_remote_config() {
	ssh "${REMOTE_HOST}" "bash -s" -- "${REMOTE_CONFIG}" "${REMOTE_DIR}" <<'EOF'
set -euo pipefail
config_path="$1"
remote_dir="$2"
expected_masterdata="${remote_dir}/deckrec/masterdata"

if [[ ! -f "${config_path}" ]]; then
	echo "[warn] remote config not found: ${config_path}"
	exit 0
fi

if grep -Fq "${expected_masterdata}" "${config_path}"; then
	echo "[ok] remote config references ${expected_masterdata}"
else
	echo "[warn] remote config does not reference ${expected_masterdata}"
fi

if grep -Fq "use_local_engine: true" "${config_path}"; then
	echo "[ok] remote config enables deck local engine"
else
	echo "[warn] remote config does not contain 'use_local_engine: true'"
fi
EOF
}

prepare_remote() {
	ssh "${REMOTE_HOST}" "mkdir -p '${REMOTE_DIR}' '${REMOTE_DIR}/deckrec' '${REMOTE_DIR}/logs'"
}

upload_binary_and_lib() {
	echo "[upload] binary"
	scp "${LOCAL_BINARY_PATH}" "${REMOTE_HOST}:${REMOTE_BINARY_PATH}.new"

	echo "[upload] libsekai_deck_recommend_c.so"
	scp "${LOCAL_LIB_PATH}" "${REMOTE_HOST}:${REMOTE_LIB_PATH}"
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
	scp "${tmp_archive}" "${REMOTE_HOST}:${REMOTE_DIR}/deckrec/masterdata.tar.tmp"

	ssh "${REMOTE_HOST}" "bash -s" -- "${REMOTE_DIR}" <<'EOF'
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
	ssh "${REMOTE_HOST}" "bash -s" -- "${REMOTE_DIR}" "${LOG_LEVEL}" <<'EOF'
set -euo pipefail
remote_dir="$1"
log_level="$2"
binary_path="${remote_dir}/haruki-server"
log_path="${remote_dir}/logs/haruki-server.out"

mv "${binary_path}.new" "${binary_path}"

chmod +x "${binary_path}"

pkill -f "${binary_path}" || true

cd "${remote_dir}"
export LD_LIBRARY_PATH="${remote_dir}${LD_LIBRARY_PATH:+:${LD_LIBRARY_PATH}}"
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
	check_remote_config
	prepare_remote
	upload_binary_and_lib
	# upload_masterdata
	restart_remote

	echo
	echo "done"
	echo "remote host: ${REMOTE_HOST}"
	echo "remote dir:  ${REMOTE_DIR}"
	echo "log tail:    ssh ${REMOTE_HOST} 'tail -f ${REMOTE_LOG_PATH}'"
}

main "$@"
