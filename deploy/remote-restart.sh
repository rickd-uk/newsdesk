#!/usr/bin/env bash
set -euo pipefail

REMOTE_DIR="${NEWSDESK_REMOTE_DIR:-/opt/newsdesk}"
SERVICE_NAME="${NEWSDESK_SERVICE:-newsdesk}"
BUILD_CMD="${NEWSDESK_BUILD_CMD:-go build -tags fts5 -o news-desk .}"
STAMP="$(date +%F-%H%M%S)"
BACKUP_KEEP="${NEWSDESK_BACKUP_KEEP:-3}"

prune_backups() {
    local prefix="$1"
    local keep="$2"
    mapfile -t files < <(find . -maxdepth 1 -type f -name "${prefix}*" -printf '%f\n' | sort)
    if (( ${#files[@]} <= keep )); then
        return
    fi
    local remove_count=$(( ${#files[@]} - keep ))
    for ((i=0; i<remove_count; i++)); do
        rm -f -- "${files[i]}"
    done
}

cd "${REMOTE_DIR}"

if [[ -f articles.db && ! -f ".deploy-auth-backup-done" ]]; then
    cp -a articles.db "articles.db.bak-${STAMP}"
    [[ -f articles.db-wal ]] && cp -a articles.db-wal "articles.db-wal.bak-${STAMP}" || true
    [[ -f articles.db-shm ]] && cp -a articles.db-shm "articles.db-shm.bak-${STAMP}" || true
    touch .deploy-auth-backup-done
    echo "created initial database backup for auth migration"
fi

eval "${BUILD_CMD}"

sudo chown www-data:www-data "${REMOTE_DIR}/news-desk"
sudo chmod 755 "${REMOTE_DIR}/news-desk"
sudo chown "$(id -un)":www-data "${REMOTE_DIR}"
sudo chmod 775 "${REMOTE_DIR}"
[[ -f "${REMOTE_DIR}/articles.db" ]] && sudo chown www-data:www-data "${REMOTE_DIR}/articles.db" || true
[[ -f "${REMOTE_DIR}/articles.db" ]] && sudo chmod 664 "${REMOTE_DIR}/articles.db" || true
[[ -f "${REMOTE_DIR}/articles.db-wal" ]] && sudo chown www-data:www-data "${REMOTE_DIR}/articles.db-wal" || true
[[ -f "${REMOTE_DIR}/articles.db-shm" ]] && sudo chown www-data:www-data "${REMOTE_DIR}/articles.db-shm" || true
[[ -f "${REMOTE_DIR}/articles.db-wal" ]] && sudo chmod 664 "${REMOTE_DIR}/articles.db-wal" || true
[[ -f "${REMOTE_DIR}/articles.db-shm" ]] && sudo chmod 664 "${REMOTE_DIR}/articles.db-shm" || true

sudo systemctl restart "${SERVICE_NAME}"
sudo systemctl --no-pager --full status "${SERVICE_NAME}"

prune_backups "articles.db.bak-" "${BACKUP_KEEP}"
prune_backups "articles.db-wal.bak-" "${BACKUP_KEEP}"
prune_backups "articles.db-shm.bak-" "${BACKUP_KEEP}"
