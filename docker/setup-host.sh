#!/usr/bin/env bash
# setup-host.sh — one-time host prep for the docker-compose stack.
#
# Frees port 53 on the host so the evilginx container (network_mode:
# host) can bind UDP/TCP 53 for autocert + phishlet DNS. Replaces
# systemd-resolved's stub resolver with static upstreams.
#
# Idempotent — safe to run twice.
#
# Usage:
#   sudo bash docker/setup-host.sh

set -euo pipefail

if [ "$(id -u)" -ne 0 ]; then
  echo "must run as root (sudo bash $0)" >&2
  exit 1
fi

echo "[1/3] Stopping and disabling systemd-resolved..."
if systemctl is-active --quiet systemd-resolved; then
  systemctl stop systemd-resolved
  systemctl disable systemd-resolved
  echo "    systemd-resolved stopped + disabled"
else
  echo "    systemd-resolved already inactive"
fi

echo "[2/3] Replacing /etc/resolv.conf with static upstreams..."
if [ -L /etc/resolv.conf ]; then
  rm /etc/resolv.conf
fi
cat <<'RESOLV' >/etc/resolv.conf
# Managed by docker/setup-host.sh - do not let systemd-resolved overwrite.
nameserver 1.1.1.1
nameserver 8.8.8.8
options timeout:2 attempts:2
RESOLV
chattr +i /etc/resolv.conf 2>/dev/null || true
echo "    /etc/resolv.conf replaced"

echo "[3/3] Verifying port 53 is free..."
if ss -tlnp | grep -q ':53 '; then
  echo "    [WARN] Something else is still bound to :53/tcp:"
  ss -tlnp | grep ':53 '
  exit 2
fi
if ss -ulnp | grep -q ':53 '; then
  echo "    [WARN] Something else is still bound to :53/udp:"
  ss -ulnp | grep ':53 '
  exit 2
fi
echo "    :53/tcp + :53/udp free"

echo ""
echo "Done. Next:"
echo "  cp configs/engagement.example.yaml evilginx-lab.yaml"
echo "  # edit evilginx-lab.yaml with your engagement metadata"
echo "  docker compose up -d --build"
