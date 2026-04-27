#!/usr/bin/env bash
# phishlab-blacklist-prune.sh — keep friendly IPs out of evilginx's
# auto-blacklist. Resolves hostnames on every run so dynamic-DNS
# entries (e.g. operator home IP via ddns.net) stay current.
#
# Install:
#   sudo install -m 0755 scripts/blacklist-prune.sh /usr/local/bin/phishlab-blacklist-prune.sh
#   sudo install -m 0644 configs/blacklist-allowlist.example.txt /etc/evilginx-lab/blacklist-allowlist.txt
#   echo '*/5 * * * * root /usr/local/bin/phishlab-blacklist-prune.sh' | sudo tee /etc/cron.d/phishlab-blacklist-prune
set -euo pipefail
ALLOW=/etc/evilginx-lab/blacklist-allowlist.txt
BL=/opt/evilginx2/state/blacklist.txt
[ -f "$ALLOW" ] || exit 0
[ -f "$BL" ] || exit 0
TMP_IPS=$(mktemp)
while IFS= read -r line; do
  line="${line%%#*}"
  line="${line//[[:space:]]/}"
  [ -z "$line" ] && continue
  if [[ "$line" =~ ^[0-9.]+$ ]] || [[ "$line" =~ ^[0-9a-fA-F:]+$ ]]; then
    echo "$line" >> "$TMP_IPS"
  else
    dig +short +time=2 +tries=1 "$line" @1.1.1.1 2>/dev/null | grep -E '^[0-9]+\.' >> "$TMP_IPS" || true
  fi
done < "$ALLOW"
TMP_BL=$(mktemp)
grep -vFf "$TMP_IPS" "$BL" > "$TMP_BL" || true
mv "$TMP_BL" "$BL"
rm -f "$TMP_IPS"
