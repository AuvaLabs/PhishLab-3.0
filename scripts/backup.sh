#!/usr/bin/env bash
# Snapshot a PhishLab engagement to a single tar.gz under /var/backups/phishlab/.
#
# Captures:
#   - engagement SQLite DB     (store.path from evilginx-lab.yaml)
#   - evilginx bbolt + state   (config.json, data.db, certmagic cache)
#   - the engagement yaml itself
#
# Usage:
#   sudo bash scripts/backup.sh [/path/to/evilginx-lab.yaml]
#
# Cron (daily at 03:30 UTC):
#   30 3 * * *  root  /home/deploy/Evilginx3PhishLab/scripts/backup.sh \
#                       /home/deploy/Evilginx3PhishLab/evilginx-lab.yaml \
#                       >> /var/log/phishlab-backup.log 2>&1

set -euo pipefail

YAML="${1:-evilginx-lab.yaml}"
if [ ! -f "$YAML" ]; then
  echo "Error: yaml not found: $YAML" >&2
  exit 1
fi

# Minimal yaml field extraction (no yq dependency). Matches simple "key: value" lines.
yaml_get() {
  local key="$1"
  awk -v k="$key" '
    {
      line = $0
      sub(/^[[:space:]]+/, "", line)
      if (line ~ "^" k ":") {
        sub(/^[^:]*:[[:space:]]*/, "", line)
        gsub(/^"|"$/, "", line)
        print line
        exit
      }
    }
  ' "$YAML"
}

ENG_ID="$(yaml_get id)"
STORE_PATH="$(yaml_get path)"
INSTALL_DIR="$(yaml_get install_dir)"

[ -z "$ENG_ID" ]     && ENG_ID="unknown"
[ -z "$STORE_PATH" ] && STORE_PATH="/var/lib/evilginx-lab/evilginx-lab.db"
[ -z "$INSTALL_DIR" ]&& INSTALL_DIR="/opt/evilginx2"

TS="$(date -u +%Y%m%dT%H%M%SZ)"
OUTDIR="/var/backups/phishlab"
OUT="$OUTDIR/${ENG_ID}-${TS}.tar.gz"
mkdir -p "$OUTDIR"

# Stage into a tmp dir so tar gets a clean tree even if some sources are missing.
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

mkdir -p "$TMP/engagement" "$TMP/evilginx-state"

cp -a "$YAML" "$TMP/engagement/evilginx-lab.yaml" 2>/dev/null || true

if [ -f "$STORE_PATH" ]; then
  if command -v sqlite3 >/dev/null 2>&1; then
    # SQLite online backup - safe even with active writers.
    sqlite3 "$STORE_PATH" ".backup '$TMP/engagement/engagement.db'"
  else
    cp -a "$STORE_PATH" "$TMP/engagement/engagement.db"
  fi
fi

if [ -d "$INSTALL_DIR/state" ]; then
  if command -v rsync >/dev/null 2>&1; then
    rsync -a --exclude='*.tmp' "$INSTALL_DIR/state/" "$TMP/evilginx-state/"
  else
    cp -a "$INSTALL_DIR/state/." "$TMP/evilginx-state/"
  fi
fi

tar -czf "$OUT" -C "$TMP" .
chmod 0600 "$OUT"

echo "[$(date -Iseconds)] backup -> $OUT  ($(du -h "$OUT" | cut -f1))"

# Retention: keep the last 14 daily snapshots per engagement.
ls -1t "$OUTDIR/${ENG_ID}-"*.tar.gz 2>/dev/null | tail -n +15 | xargs -r rm -f --
