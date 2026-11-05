#!/usr/bin/env bash
# Runs on the Hetzner box itself, from root's crontab (same style as
# neurocrow-hooks' scripts/backup-hetzner-db.sh). Takes an online SQLite backup
# of PocketBase's data.db, gzips it into /opt/backups/instantappoint/, and
# prunes backups older than RETENTION_DAYS.
#
# Only data.db: auxiliary.db holds PocketBase's request logs, which are
# disposable. Uploaded files under pb_data/storage are not backed up yet; the
# app does not store any today.
#
# Needs the host's sqlite3 CLI: the image is distroless, so there is nothing to
# `docker exec` into. sqlite3's .backup uses SQLite's online backup API, which
# is safe while PocketBase is writing (WAL mode). It runs as the container's
# uid 65532 so it never leaves root-owned -wal/-shm files the app could not
# open afterwards.
#
# NOTE: these backups sit on the same disk as the data. They cover a bad
# migration or a deleted record, not the loss of the box.
set -euo pipefail

DATA_DIR=/opt/apps/instantappoint/data/pb_data
DB="$DATA_DIR/data.db"
BACKUP_DIR=/opt/backups/instantappoint
RETENTION_DAYS=14
TIMESTAMP=$(date -u +%Y%m%d_%H%M%S)
TMP_COPY="$DATA_DIR/.backup_${TIMESTAMP}.db"
OUT_FILE="$BACKUP_DIR/data_${TIMESTAMP}.db.gz"

mkdir -p "$BACKUP_DIR"
chmod 700 "$BACKUP_DIR"
trap 'rm -f "$TMP_COPY"' EXIT

setpriv --reuid=65532 --regid=65532 --clear-groups \
  sqlite3 "$DB" ".backup '$TMP_COPY'"
gzip -c "$TMP_COPY" > "$OUT_FILE"
chmod 600 "$OUT_FILE"

echo "$(date -u +%FT%TZ) backup written: $OUT_FILE ($(du -h "$OUT_FILE" | cut -f1))"

find "$BACKUP_DIR" -name 'data_*.db.gz' -mtime "+${RETENTION_DAYS}" -print -delete
