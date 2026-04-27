#!/bin/bash
# ==== PhishLab 3.0 Installer (Evilginx3 v3.3.0 + Gophish + Mailhog + analyst dashboard) ====
# Idempotent. Re-run safely. Tested on Ubuntu 22.04 / 24.04.
#
# Usage:
#   bash install.sh login.example.com
#   bash install.sh login.example.com --with-ops-panel
#
# Flags:
#   --with-ops-panel   Provision nginx :8443 reverse proxy with basic auth so
#                      analysts can reach Gophish, Mailhog, and the engagement
#                      dashboard from a workstation without an SSH tunnel.
#                      Reads OPS_PANEL_USER (default: csoc) and OPS_PANEL_PASS
#                      (prompted if unset) from env. Generates a self-signed cert
#                      under /etc/nginx/ssl/phishlab-ops/ which you should replace
#                      with a real Let's Encrypt cert (see docs/DEPLOY.md).

set -euo pipefail

cleanup() {
  echo ""
  echo "Installation interrupted or failed. Partial install may exist."
  echo "Re-run this script to continue or fix issues manually."
}
trap cleanup ERR

# ==== Resolve SCRIPT_DIR before any cd ====
# Must compute up front: later steps cd into /opt and other places, after which
# dirname "$BASH_SOURCE" would resolve relative to the wrong CWD.
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"

# ==== Parse args ====
WITH_OPS_PANEL=false
DOMAIN=""
for arg in "$@"; do
  case "$arg" in
    --with-ops-panel) WITH_OPS_PANEL=true ;;
    --help|-h)
      sed -n '1,15p' "$0"
      exit 0
      ;;
    -*) echo "Unknown flag: $arg" >&2; exit 1 ;;
    *)  [ -z "$DOMAIN" ] && DOMAIN="$arg" ;;
  esac
done

if [ -z "$DOMAIN" ]; then
  read -rp "Enter your phishing domain (e.g. login.example.com): " DOMAIN
fi
if [ -z "$DOMAIN" ]; then
  echo "Error: Domain cannot be empty."
  exit 1
fi

# Derive the apex (drop a single leading host label, e.g. login.example.com -> example.com)
APEX="${DOMAIN#*.}"
if [ "$APEX" = "$DOMAIN" ]; then APEX="$DOMAIN"; fi

# ==== Variables ====
EMAIL="admin@$DOMAIN"
GOPHISH_PORT=8800
MAILHOG_UI_PORT=8025
MAILHOG_SMTP_PORT=1025
DASHBOARD_PORT=9000
GOPHISH_VERSION="0.12.1"
GO_VERSION="1.22.3"
EVILGINX_DIR="/opt/evilginx2"
EVILGINX_STATE="$EVILGINX_DIR/state"
PHISHLETS_PATH="$EVILGINX_DIR/phishlets"
SERVICE_USER="phishlab"

# Resolve public IP (force IPv4; ifconfig.me defaults to v6 on dual-stack hosts)
echo "Resolving public IP..."
PUBLIC_IP="$(curl -4sf ifconfig.me || curl -4sf icanhazip.com || true)"
if [ -z "$PUBLIC_IP" ]; then
  echo "Error: Could not determine public IP address."
  exit 1
fi
echo "Public IP: $PUBLIC_IP"

is_installed() { command -v "$1" &>/dev/null; }

# ==== Update & Install Base Packages ====
echo "Installing base packages..."
apt update && apt upgrade -y
apt install -y git make curl unzip ufw build-essential ca-certificates \
  gnupg lsb-release libcap2-bin net-tools jq wget bind9-host

# ==== Create Service User ====
if ! id "$SERVICE_USER" &>/dev/null; then
  useradd -r -s /usr/sbin/nologin -d /opt "$SERVICE_USER"
fi

# ==== Install Go ====
if ! is_installed go || [[ "$(go version 2>/dev/null)" != *"$GO_VERSION"* ]]; then
  echo "Installing Go $GO_VERSION..."
  cd /tmp
  curl -LO "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
  rm -rf /usr/local/go && tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
  rm -f "go${GO_VERSION}.linux-amd64.tar.gz"
else
  echo "Go $GO_VERSION already installed, skipping."
fi

export PATH=$PATH:/usr/local/go/bin
echo 'export PATH=$PATH:/usr/local/go/bin' > /etc/profile.d/go.sh
chmod +x /etc/profile.d/go.sh
grep -qxF 'export PATH=$PATH:/usr/local/go/bin' /root/.bashrc 2>/dev/null \
  || echo 'export PATH=$PATH:/usr/local/go/bin' >> /root/.bashrc

# ==== Install Evilginx3 (v3.3.0) ====
if [ ! -f "$EVILGINX_DIR/dist/evilginx" ]; then
  echo "Building Evilginx3 v3.3.0..."
  cd /opt
  rm -rf "$EVILGINX_DIR"
  git clone --branch v3.3.0 https://github.com/kgretzky/evilginx2.git "$EVILGINX_DIR"
  cd "$EVILGINX_DIR"
  mkdir -p dist
  go build -o dist/evilginx main.go
  setcap cap_net_bind_service=+ep dist/evilginx
else
  echo "Evilginx3 binary already exists, skipping build."
fi

# Pre-create writable state dir owned by root (evilginx unit runs as root for
# privileged port binding; HOME is overridden to this dir so cert storage
# (CertMagic uses $HOME/.local/share/certmagic/) lands here, not in /root).
mkdir -p "$EVILGINX_STATE"
chown -R root:root "$EVILGINX_STATE"

# ==== Copy Bundled Phishlets ====
if [ -d "$SCRIPT_DIR/phishlets" ]; then
  echo "Copying bundled phishlets..."
  cp "$SCRIPT_DIR"/phishlets/*.yaml "$PHISHLETS_PATH/" 2>/dev/null || true
  echo "Phishlets installed: $(ls "$PHISHLETS_PATH"/*.yaml 2>/dev/null | wc -l) files"
fi

# ==== Evilginx Setup Commands Reference (v3 syntax) ====
cat <<EOF > /root/evilginx_setup_commands.txt
# Run these commands inside the Evilginx interactive prompt:
#   sudo HOME=$EVILGINX_STATE $EVILGINX_DIR/dist/evilginx \\
#     -c $EVILGINX_STATE -p $PHISHLETS_PATH
#
# Then paste each line below (Evilginx3 v3 syntax):

config domain $APEX
config ipv4 external $PUBLIC_IP
config autocert on
phishlets hostname o365 $APEX
phishlets enable o365

# After phishlets enable, create a lure to get the entry URL:
lures create o365
lures get-url 0
EOF

echo "Evilginx setup commands saved to /root/evilginx_setup_commands.txt"

# ==== Build Evilginx-Lab ====
LABDIR="$SCRIPT_DIR"
if [ ! -f /usr/local/bin/evilginx-lab ] || [ "$LABDIR/cmd/evilginx-lab/main.go" -nt /usr/local/bin/evilginx-lab ]; then
  echo "Building Evilginx-Lab..."
  cd "$LABDIR"
  apt install -y gcc libsqlite3-dev 2>/dev/null || true
  go mod download
  mkdir -p dist
  CGO_ENABLED=1 go build -ldflags "-s -w" -o dist/evilginx-lab ./cmd/evilginx-lab
  install -m 755 dist/evilginx-lab /usr/local/bin/evilginx-lab
  mkdir -p /var/lib/evilginx-lab
  chown "$SERVICE_USER":"$SERVICE_USER" /var/lib/evilginx-lab
  echo "evilginx-lab binary installed to /usr/local/bin/evilginx-lab"
else
  echo "evilginx-lab binary is up to date, skipping build."
fi

# ==== Install Gophish ====
GOPHISH_DIR="/opt/gophish"
GOPHISH_URL="https://github.com/gophish/gophish/releases/download/v${GOPHISH_VERSION}/gophish-v${GOPHISH_VERSION}-linux-64bit.zip"

if [ ! -f "$GOPHISH_DIR/gophish" ]; then
  echo "Installing Gophish v${GOPHISH_VERSION}..."
  cd /opt
  curl -LO "$GOPHISH_URL"
  unzip -o "gophish-v${GOPHISH_VERSION}-linux-64bit.zip" -d gophish
  rm -f "gophish-v${GOPHISH_VERSION}-linux-64bit.zip"
  chmod +x "$GOPHISH_DIR/gophish"
else
  echo "Gophish already installed, skipping."
fi

# Configure Gophish:
#   - admin server bound to 127.0.0.1:$GOPHISH_PORT (reach via SSH tunnel or ops panel)
#   - phish server bound to 127.0.0.1:8081 - DEFAULT 0.0.0.0:80 collides with
#     Evilginx (which owns 80/443) and would also fail since we run gophish
#     as the unprivileged $SERVICE_USER. Templates should embed Evilginx URLs
#     directly; gophish phish_server is kept up for tracking-pixel callbacks.
cd "$GOPHISH_DIR"
jq --arg admin "127.0.0.1:$GOPHISH_PORT" \
   '.admin_server.listen_url = $admin | .admin_server.use_tls = false
    | .phish_server.listen_url = "127.0.0.1:8081"
    | .phish_server.use_tls = false' \
   config.json > config.json.tmp && mv config.json.tmp config.json

chown -R "$SERVICE_USER":"$SERVICE_USER" "$GOPHISH_DIR"

# ==== Install Mailhog ====
if [ ! -f /usr/local/bin/mailhog ]; then
  echo "Installing Mailhog..."
  wget -O /usr/local/bin/mailhog \
    https://github.com/mailhog/MailHog/releases/download/v1.0.1/MailHog_linux_amd64
  chmod +x /usr/local/bin/mailhog
else
  echo "Mailhog already installed, skipping."
fi

# ==== Setup UFW Firewall ====
echo "Configuring firewall..."
ufw allow 22/tcp
ufw allow 80/tcp
ufw allow 443/tcp
if "$WITH_OPS_PANEL"; then
  ufw allow 8443/tcp comment "PhishLab ops panel"
fi
ufw --force enable

# ==== Systemd Services ====

# Evilginx3 service.
#
# Why this wrapper rather than a plain ExecStart=evilginx:
#   * evilginx is a TUI that calls readline on stdin. Without a tty stdin it
#     EOFs immediately and the binary exits cleanly, killing the proxy goroutines.
#   * sleep infinity provides a stdin that has no data and never EOFs, so
#     readline blocks indefinitely and the binary stays up.
#   * exec REPLACES bash with evilginx so evilginx becomes the unit main PID.
#     The old (sleep infinity | evilginx) pipeline kept bash + sleep alive even
#     after evilginx died; systemd reported is-active=true on a dead proxy and
#     Restart=always never fired. With exec, evilginx death == unit death.
cat <<EOF > /etc/systemd/system/evilginx.service
[Unit]
Description=Evilginx3 Phishing Proxy
After=network.target

[Service]
Type=simple
User=root
Group=root
WorkingDirectory=$EVILGINX_DIR
Environment=HOME=$EVILGINX_STATE
ExecStartPre=/bin/mkdir -p $EVILGINX_STATE
ExecStart=/bin/bash -c 'exec $EVILGINX_DIR/dist/evilginx -c $EVILGINX_STATE -p $PHISHLETS_PATH < <(exec sleep infinity)'
KillMode=control-group
Restart=always
RestartSec=5
LimitNOFILE=65535

[Install]
WantedBy=multi-user.target
EOF

# Gophish service
cat <<EOF > /etc/systemd/system/gophish.service
[Unit]
Description=Gophish Phishing Server
After=network.target

[Service]
Type=simple
User=$SERVICE_USER
WorkingDirectory=$GOPHISH_DIR
ExecStart=$GOPHISH_DIR/gophish
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Mailhog service
cat <<EOF > /etc/systemd/system/mailhog.service
[Unit]
Description=Mailhog SMTP Testing Server
After=network.target

[Service]
ExecStart=/usr/local/bin/mailhog \
  -smtp-bind-addr=127.0.0.1:$MAILHOG_SMTP_PORT \
  -api-bind-addr=127.0.0.1:$MAILHOG_UI_PORT \
  -ui-bind-addr=127.0.0.1:$MAILHOG_UI_PORT
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

# Evilginx-Lab dashboard service.
# Symlink legacy /opt/evilginx2/data.db -> $EVILGINX_STATE/data.db so the
# poller (which reads from the install_dir-relative path) finds the bbolt file
# in its new state-dir location.
ln -sfn "$EVILGINX_STATE/data.db" "$EVILGINX_DIR/data.db" || true

cat <<EOF > /etc/systemd/system/evilginx-lab.service
[Unit]
Description=Evilginx-Lab Dashboard
After=network.target evilginx.service gophish.service

[Service]
Type=simple
User=root
WorkingDirectory=$LABDIR
ExecStart=/usr/local/bin/evilginx-lab deploy -c $LABDIR/evilginx-lab.yaml
Restart=always
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

systemctl daemon-reload
systemctl enable --now evilginx gophish mailhog

# ==== Optional: ops panel (nginx :8443 + basic auth) ====
install_ops_panel() {
  echo "Installing ops panel (nginx :8443 + basic auth)..."
  apt install -y nginx apache2-utils

  local OP_USER="${OPS_PANEL_USER:-csoc}"
  local OP_PASS="${OPS_PANEL_PASS:-}"
  if [ -z "$OP_PASS" ]; then
    read -rsp "Ops panel password for user '$OP_USER': " OP_PASS
    echo
  fi
  if [ -z "$OP_PASS" ]; then
    echo "Error: ops panel password cannot be empty (set OPS_PANEL_PASS or supply at prompt)."
    return 1
  fi

  htpasswd -cb /etc/nginx/.htpasswd-phishlab "$OP_USER" "$OP_PASS"

  # Self-signed cert as a placeholder. Replace with a real Let's Encrypt cert
  # using certbot --dns-ionos (or your DNS plugin) - see docs/DEPLOY.md.
  mkdir -p /etc/nginx/ssl/phishlab-ops
  if [ ! -f /etc/nginx/ssl/phishlab-ops/fullchain.pem ]; then
    openssl req -x509 -nodes -days 365 -newkey rsa:2048 \
      -keyout /etc/nginx/ssl/phishlab-ops/key.pem \
      -out /etc/nginx/ssl/phishlab-ops/fullchain.pem \
      -subj "/CN=ops.${APEX}" 2>/dev/null
  fi

  # Hostnames the analyst will browse to (must have A records pointing here).
  local GOPHISH_HOST="gophish.${APEX}"
  local MAILHOG_HOST="mailhog.${APEX}"
  local DASH_HOST="dashboard.${APEX}"

  cat <<NGINX > /etc/nginx/sites-available/phishlab-ops
ssl_certificate     /etc/nginx/ssl/phishlab-ops/fullchain.pem;
ssl_certificate_key /etc/nginx/ssl/phishlab-ops/key.pem;

# Gophish admin - protected by Gophish's own login
server {
    listen 8443 ssl;
    server_name $GOPHISH_HOST;
    location / {
        proxy_pass         http://127.0.0.1:$GOPHISH_PORT;
        proxy_set_header   Host              \$host;
        proxy_set_header   X-Real-IP         \$remote_addr;
        proxy_set_header   X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto https;
        proxy_http_version 1.1;
        proxy_set_header   Upgrade           \$http_upgrade;
        proxy_set_header   Connection        keep-alive;
    }
}

# Mailhog - no built-in auth, protected by basic auth
server {
    listen 8443 ssl;
    server_name $MAILHOG_HOST;
    auth_basic           "PhishLab Ops";
    auth_basic_user_file /etc/nginx/.htpasswd-phishlab;
    location / {
        proxy_pass         http://127.0.0.1:$MAILHOG_UI_PORT;
        proxy_set_header   Host              \$host;
        proxy_set_header   X-Real-IP         \$remote_addr;
        proxy_set_header   X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_http_version 1.1;
        proxy_set_header   Upgrade           \$http_upgrade;
        proxy_set_header   Connection        "upgrade";
    }
}

# Engagement dashboard - protected by basic auth
server {
    listen 8443 ssl;
    server_name $DASH_HOST;
    auth_basic           "PhishLab Ops";
    auth_basic_user_file /etc/nginx/.htpasswd-phishlab;
    location / {
        proxy_pass         http://127.0.0.1:$DASHBOARD_PORT;
        proxy_set_header   Host              \$host;
        proxy_set_header   X-Real-IP         \$remote_addr;
        proxy_set_header   X-Forwarded-For   \$proxy_add_x_forwarded_for;
        proxy_set_header   X-Forwarded-Proto https;
    }
}
NGINX

  ln -sfn /etc/nginx/sites-available/phishlab-ops /etc/nginx/sites-enabled/phishlab-ops
  # Remove default vhost so nginx doesn't bind :80 (evilginx needs :80 for autocert)
  rm -f /etc/nginx/sites-enabled/default

  if ! nginx -t 2>&1; then
    echo "Error: nginx config test failed."
    return 1
  fi
  systemctl restart nginx

  echo "Ops panel ready. Once DNS A records exist for"
  echo "  $GOPHISH_HOST  $MAILHOG_HOST  $DASH_HOST  -> $PUBLIC_IP"
  echo "browse to https://$DASH_HOST:8443/ (accept the self-signed cert warning,"
  echo "or replace fullchain.pem/key.pem with a Let's Encrypt cert)."
}

if "$WITH_OPS_PANEL"; then
  install_ops_panel || echo "[WARN] ops panel install failed; rerun with --with-ops-panel after fixing"
fi

# ==== Post-Install Verification ====
printf "\nVerifying service availability...\n"

for svc in evilginx gophish mailhog; do
  if systemctl is-active --quiet "$svc"; then
    echo "[OK] $svc is running"
  else
    echo "[FAIL] $svc failed to start - check: journalctl -u $svc"
  fi
done

# Wait up to 60s for evilginx to actually bind :443 (the systemd is-active check
# can lie if the wrapper bug ever returns; this catches it).
printf "Waiting for evilginx to bind :443"
for _ in $(seq 1 12); do
  if ss -tlnp 2>/dev/null | grep -q ':443'; then
    echo " OK"
    break
  fi
  printf "."
  sleep 5
done
if ! ss -tlnp 2>/dev/null | grep -q ':443'; then
  echo
  echo "[FAIL] evilginx did not bind :443 within 60s. Recent logs:"
  journalctl -u evilginx -n 30 --no-pager || true
  echo "Note: phishlet must be enabled before evilginx binds :443. See"
  echo "/root/evilginx_setup_commands.txt for the bootstrap commands."
fi

printf "\nActive listeners:\n"
ss -tlnp | grep -E ":(80|443|$GOPHISH_PORT|$MAILHOG_UI_PORT|$MAILHOG_SMTP_PORT|$DASHBOARD_PORT|8443)" || true

# ==== Capture Gophish Initial Password ====
printf "\nWaiting for Gophish to generate initial password...\n"
sleep 5
GOPHISH_INITIAL_PASS=$(journalctl -u gophish --no-pager -n 100 | grep -oP 'Please login with the username admin and the password \K\S+' | head -1 || true)

# ==== Completion Output ====
printf "\n========================================\n"
printf "  Setup Complete\n"
printf "========================================\n\n"

echo "Domain:       $DOMAIN  (apex: $APEX)"
echo "Public IP:    $PUBLIC_IP"
echo ""
echo "Evilginx3:    running as systemd service 'evilginx'"
echo "  Binary:     $EVILGINX_DIR/dist/evilginx"
echo "  State dir:  $EVILGINX_STATE   (data.db, certs, config.json live here)"
echo "  Setup:      Paste commands from /root/evilginx_setup_commands.txt"
echo ""
echo "Gophish:      http://127.0.0.1:$GOPHISH_PORT"
echo "  SSH tunnel: ssh -L $GOPHISH_PORT:127.0.0.1:$GOPHISH_PORT root@$PUBLIC_IP"
echo "  Username:   admin"
if [ -n "$GOPHISH_INITIAL_PASS" ]; then
  echo "  Password:   $GOPHISH_INITIAL_PASS  (initial - change on first login)"
else
  echo "  Password:   journalctl -u gophish | grep password"
fi
echo ""
echo "Mailhog:      http://127.0.0.1:$MAILHOG_UI_PORT"
echo "  SSH tunnel: ssh -L $MAILHOG_UI_PORT:127.0.0.1:$MAILHOG_UI_PORT deploy@$PUBLIC_IP"
echo "  SMTP:       127.0.0.1:$MAILHOG_SMTP_PORT (configure as Gophish sending profile)"
echo ""
echo "Dashboard:    http://127.0.0.1:$DASHBOARD_PORT"
echo "  SSH tunnel: ssh -L $DASHBOARD_PORT:127.0.0.1:$DASHBOARD_PORT root@$PUBLIC_IP"
if "$WITH_OPS_PANEL"; then
echo "  Ops panel:  https://dashboard.$APEX:8443  (basic auth: $OP_USER)"
fi
echo ""
echo "Quick Start:"
echo "  1. cp configs/engagement.example.yaml evilginx-lab.yaml"
echo "  2. Edit evilginx-lab.yaml with your engagement details"
echo "  3. evilginx-lab init   -c evilginx-lab.yaml"
echo "  4. evilginx-lab deploy -c evilginx-lab.yaml"
