#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
REMOTE_HOST="${NEWSDESK_HOST:-${1:-hz-hel}}"
REMOTE_DIR="${NEWSDESK_REMOTE_DIR:-/opt/newsdesk}"
REMOTE_SERVICE="${NEWSDESK_SERVICE:-newsdesk}"
BUILD_CMD="${NEWSDESK_BUILD_CMD:-go build -tags fts5 -o news-desk .}"

rsync -av \
    --exclude='.git' \
    --exclude='.codex' \
    --exclude='articles.db' \
    --exclude='articles.db-wal' \
    --exclude='articles.db-shm' \
    --exclude='articles.db.bak*' \
    --exclude='articles.db-shm.bak*' \
    --exclude='articles.db-wal.bak*' \
    --exclude='*.bak' \
    --exclude='db-backups' \
    --exclude='db-rescue-*' \
    --exclude='news-desk' \
    --exclude='article-viewer' \
    --exclude='*.test' \
    "${ROOT_DIR}/" \
    "${REMOTE_HOST}:${REMOTE_DIR}/"

ssh -t "${REMOTE_HOST}" \
    "NEWSDESK_REMOTE_DIR='${REMOTE_DIR}' NEWSDESK_SERVICE='${REMOTE_SERVICE}' NEWSDESK_BUILD_CMD='${BUILD_CMD}' bash '${REMOTE_DIR}/deploy/remote-restart.sh'"
