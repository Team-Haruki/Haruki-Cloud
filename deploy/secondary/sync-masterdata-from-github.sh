#!/usr/bin/env bash
set -Eeuo pipefail

MASTER_ROOT=${MASTERDATA_ROOT:-/data/HarukiServices/data/masterdata}
GITHUB_OWNER=${HARUKI_MASTERDATA_GITHUB_OWNER:-Team-Haruki}
GITHUB_BRANCH=${HARUKI_MASTERDATA_GITHUB_BRANCH:-main}
DECK_SERVICE_URL=${DECK_SERVICE_URL:-http://127.0.0.1:3000}
FORCE_RELOAD=${HARUKI_MASTERDATA_FORCE_RELOAD:-false}

export HOME=${HOME:-/root}

repos=(
  haruki-sekai-master
  haruki-sekai-en-master
  haruki-sekai-sc-master
  haruki-sekai-tc-master
  haruki-sekai-kr-master
)

log() {
  printf "[%s] %s\n" "$(date -Is)" "$*"
}

repo_url() {
  printf "https://github.com/%s/%s.git" "$GITHUB_OWNER" "$1"
}

mark_safe_directory() {
  local dir="$1"
  if ! git config --global --get-all safe.directory | grep -Fxq "$dir"; then
    git config --global --add safe.directory "$dir"
  fi
}

sync_repo() {
  local repo="$1"
  local dest="$MASTER_ROOT/$repo"
  local url before after stash_output backup

  url="$(repo_url "$repo")"
  mkdir -p "$MASTER_ROOT"

  if [[ ! -d "$dest/.git" ]]; then
    if [[ -e "$dest" ]]; then
      backup="${dest}.backup-$(date +%Y%m%d%H%M%S)"
      log "$repo existing non-git directory moved to $backup"
      mv "$dest" "$backup"
    fi
    log "$repo cloning $url branch=$GITHUB_BRANCH"
    git clone --branch "$GITHUB_BRANCH" --single-branch "$url" "$dest"
    mark_safe_directory "$dest"
    after="$(git -C "$dest" rev-parse HEAD)"
    log "$repo ready $(git -C "$dest" log -1 --format='%h %s')"
    return 0
  fi

  mark_safe_directory "$dest"
  git -C "$dest" remote set-url origin "$url"
  before="$(git -C "$dest" rev-parse HEAD 2>/dev/null || true)"

  log "$repo fetching origin/$GITHUB_BRANCH"
  git -C "$dest" fetch --prune origin "$GITHUB_BRANCH"

  if [[ -n "$(git -C "$dest" status --porcelain)" ]]; then
    stash_output="$(
      git -C "$dest" stash push -u \
        -m "pre-github-masterdata-sync-$(date +%Y%m%d%H%M%S)" || true
    )"
    log "$repo stashed local changes: $stash_output"
  fi

  git -C "$dest" checkout -B "$GITHUB_BRANCH" "origin/$GITHUB_BRANCH"
  git -C "$dest" reset --hard "origin/$GITHUB_BRANCH" >/dev/null
  git -C "$dest" clean -fd >/dev/null

  after="$(git -C "$dest" rev-parse HEAD)"
  if [[ "$before" != "$after" ]]; then
    log "$repo updated $before -> $after"
  else
    log "$repo unchanged $(git -C "$dest" log -1 --format='%h %s')"
  fi
}

reload_masterdata() {
  local region
  for region in jp en cn tw kr; do
    curl -fsS --max-time 30 \
      -H "Content-Type: application/json" \
      -d "{\"base_dir\":\"/masterdata\",\"region\":\"$region\"}" \
      "$DECK_SERVICE_URL/update/masterdata" >/dev/null || \
      log "deck-service masterdata reload failed region=$region"
  done
}

musicmetas_filename() {
  case "$1" in
    cn) printf "music_metas-cn.json" ;;
    tw) printf "music_metas-tc.json" ;;
    en) printf "music_metas-en.json" ;;
    kr) printf "music_metas-kr.json" ;;
    *) printf "music_metas.json" ;;
  esac
}

reload_musicmetas() {
  local region file
  for region in jp en cn tw kr; do
    file="$(musicmetas_filename "$region")"
    [[ -s "$MASTER_ROOT/$file" ]] || continue
    curl -fsS --max-time 30 \
      -H "Content-Type: application/json" \
      -d "{\"file_path\":\"/masterdata/$file\",\"region\":\"$region\"}" \
      "$DECK_SERVICE_URL/update/musicmetas" >/dev/null || \
      log "deck-service musicmetas reload failed region=$region file=$file"
  done
}

verify_supplements() {
  local file missing=false
  for file in \
    worldBloomSupportDeckBonusesWL1.json \
    worldBloomSupportDeckBonusesWL2.json \
    worldBloomSupportDeckBonusesWL3.json; do
    if [[ ! -s "$MASTER_ROOT/$file" ]]; then
      log "missing masterdata supplement $MASTER_ROOT/$file"
      missing=true
    fi
  done
  [[ "$missing" == "false" ]]
}

changed=false
for repo in "${repos[@]}"; do
  before="$(git -C "$MASTER_ROOT/$repo" rev-parse HEAD 2>/dev/null || true)"
  sync_repo "$repo"
  after="$(git -C "$MASTER_ROOT/$repo" rev-parse HEAD 2>/dev/null || true)"
  [[ "$before" == "$after" ]] || changed=true
done

if ! verify_supplements; then
  log "supplement files should live in $MASTER_ROOT"
fi

if [[ "$changed" == "true" || "$FORCE_RELOAD" == "true" ]]; then
  log "reloading deck-service masterdata"
  reload_masterdata
  reload_musicmetas
fi

log "github masterdata sync done changed=$changed root=$MASTER_ROOT"
