#!/usr/bin/env bash
# prepare-release.sh — assemble the open-source release folder
#
# Usage:
#   ./scripts/prepare-release.sh [output-dir]
#
# Default output: <repo-root>/release/
# The script is idempotent; re-running it refreshes the output folder.

set -euo pipefail

REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
OUT="${1:-"$REPO_ROOT/release"}"

echo "==> Preparing open-source release in: $OUT"

# Clean previous run
rm -rf "$OUT"
mkdir -p "$OUT"

rsync -a --delete \
  --exclude='.git/' \
  --exclude='.DS_Store' \
  --exclude='.idea/' \
  --exclude='.vscode/' \
  --exclude='.claude/' \
  --exclude='.env' \
  --exclude='release/' \
  \
  --exclude='sshkey' \
  --exclude='client.json' \
  --exclude='haruki-cloud.yaml' \
  --exclude='haruki-cloud' \
  --exclude='haruki-server-linux' \
  --exclude='haruki-server-linux.gz' \
  --exclude='server' \
  --exclude='importer' \
  --exclude='importer-linux-amd64' \
  --exclude='main.log' \
  --exclude='test.go' \
  --exclude='test.py' \
  --exclude='test_auth' \
  --exclude='IMG_7736.png' \
  --exclude='music_metas.json' \
  --exclude='collections.suite.json' \
  --exclude='schema_info.json' \
  \
  --exclude='data/' \
  --exclude='Data/' \
  --exclude='character/' \
  --exclude='music/' \
  --exclude='drawing/' \
  --exclude='exports/' \
  --exclude='tmp/' \
  --exclude='build/' \
  --exclude='builds/' \
  --exclude='out/' \
  \
  "$REPO_ROOT/" "$OUT/"

echo "==> Writing release .gitignore"
cat > "$OUT/.gitignore" <<'EOF'
# Runtime / local config
haruki-cloud.yaml
.env
haruki-db-configs.yaml

# Secrets
sshkey
client.json

# Binaries
haruki-cloud
haruki-server-linux
haruki-server-linux.gz
server
importer
importer-linux-amd64

# Game / asset data (mounted at runtime)
data/
Data/
character/
music/
drawing/
exports/
tmp/

# Local test artifacts
collections.suite.json
music_metas.json
main.log
*.out
*.test

# IDE / OS
.idea/
.vscode/
.DS_Store
EOF

echo "==> Done. Files in $OUT:"
find "$OUT" -not -path '*/.git/*' -type f | sort | sed "s|$OUT/||"
