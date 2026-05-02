#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REMOTE_HOST="${NEWSDESK_HOST:-${1:-hz-hel}}"
REMOTE_DIR="${NEWSDESK_REMOTE_DIR:-/opt/newsdesk}"
REMOTE_SERVICE="${NEWSDESK_SERVICE:-newsdesk}"
LOCAL_DB="${NEWSDESK_LOCAL_DB:-${ROOT_DIR}/articles.db}"
REMOTE_TMP_DB="${REMOTE_DIR}/articles.db.new"
STAMP="$(date +%F-%H%M%S)"
BACKUP_KEEP="${NEWSDESK_BACKUP_KEEP:-3}"

if [[ ! -f "${LOCAL_DB}" ]]; then
    echo "database not found: ${LOCAL_DB}"
    exit 1
fi

scp "${LOCAL_DB}" "${REMOTE_HOST}:${REMOTE_TMP_DB}"

ssh -t "${REMOTE_HOST}" "bash -lc '
set -euo pipefail
cd \"${REMOTE_DIR}\"
BACKUP_KEEP=\"${BACKUP_KEEP}\"
prune_backups() {
    local prefix=\"\$1\"
    local keep=\"\$2\"
    mapfile -t files < <(find . -maxdepth 1 -type f -name \"\${prefix}*\" -printf \"%f\n\" | sort)
    if (( \${#files[@]} <= keep )); then
        return
    fi
    local remove_count=\$(( \${#files[@]} - keep ))
    for ((i=0; i<remove_count; i++)); do
        rm -f -- \"\${files[i]}\"
    done
}
sudo systemctl stop \"${REMOTE_SERVICE}\"
if [[ -f articles.db ]]; then mv articles.db \"articles.db.pre-sync.bak-${STAMP}\"; fi
if [[ -f articles.db-wal ]]; then mv articles.db-wal \"articles.db-wal.pre-sync.bak-${STAMP}\"; fi
if [[ -f articles.db-shm ]]; then mv articles.db-shm \"articles.db-shm.pre-sync.bak-${STAMP}\"; fi
mv articles.db.new articles.db
sudo chown \"\$(id -un)\":www-data .
sudo chmod 775 .
sudo chown www-data:www-data articles.db
sudo chmod 664 articles.db
rm -f articles.db-wal articles.db-shm
sudo systemctl start \"${REMOTE_SERVICE}\"
sudo systemctl --no-pager --full status \"${REMOTE_SERVICE}\"
prune_backups \"articles.db.pre-sync.bak-\" \"\$BACKUP_KEEP\"
prune_backups \"articles.db-wal.pre-sync.bak-\" \"\$BACKUP_KEEP\"
prune_backups \"articles.db-shm.pre-sync.bak-\" \"\$BACKUP_KEEP\"
'"
