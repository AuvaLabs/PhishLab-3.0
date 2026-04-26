# Deployment Runbook — PhishLab 3.0 on cyb3rdefence.com

End-to-end procedure for standing up the lab on a fresh Ubuntu host and
wiring it to the `cyb3rdefence.com` engagement domain managed via IONOS DNS.

## 1. Topology

```
             cyb3rdefence.com (managed at IONOS — operator API key)
                       │
   apex / login / phish / securephish[01|02]   ──A──>  91.98.47.193
   (other CNAMEs to apex)

   PhishLab host: 91.98.47.193  (hostname: cyb3rdefence-project)
   ┌─────────────────────────────────────────────────────────────┐
   │  Port 22   sshd                                             │
   │  Port 80   evilginx (HTTP / autocert)                       │
   │  Port 443  evilginx (TLS phishing proxy)                    │
   │  Port 8025 mailhog UI/API   (loopback only)                 │
   │  Port 8800 gophish admin    (loopback only)                 │
   │  Port 8443 evilginx-lab     (loopback only)                 │
   │  Port 1025 mailhog SMTP     (loopback only)                 │
   └─────────────────────────────────────────────────────────────┘
```

Loopback-only services are reached via SSH tunnel from the operator workstation:

```bash
ssh -L 8800:127.0.0.1:8800 \
    -L 8025:127.0.0.1:8025 \
    -L 8443:127.0.0.1:8443 \
    deploy@91.98.47.193
```

## 2. DNS prerequisites (IONOS API)

The phishing subdomains used by PhishLab must resolve to the host IP. As of
2026-04-26 the following A records are in place, TTL 60s:

| Hostname | Type | Target |
|---|---|---|
| `cyb3rdefence.com` | A | 91.98.47.193 |
| `login.cyb3rdefence.com` | CNAME | apex |
| `phish.cyb3rdefence.com` | A | 91.98.47.193 |
| `securephish.cyb3rdefence.com` | A | 91.98.47.193 |
| `securephish01.cyb3rdefence.com` | A | 91.98.47.193 |
| `securephish02.cyb3rdefence.com` | A | 91.98.47.193 |

`oob.cyb3rdefence.com` and `ns1.oob` / `ns2.oob` belong to the **redblue**
project (security scanner OOB callbacks) and are managed there — out of
scope for PhishLab.

Spot-check from any resolver:

```bash
set -a; source ~/.config/cyb3rdefence/ionos.env; set +a
ZONE_ID=7634f821-1077-11ef-acfb-0a5864440c92
curl -sS -H "X-API-Key: $IONOS_API_KEY" \
  "https://api.hosting.ionos.com/dns/v1/zones/$ZONE_ID" \
  | jq -r '.records[] | select(.type=="A") | "\(.name)\t\(.content)"'
```

## 3. Install (one shot)

On the host as `deploy` (passwordless sudo configured — see prerequisites
note below):

```bash
sudo bash install.sh login.cyb3rdefence.com
```

What this does:
- `apt update && upgrade -y`, base packages
- Creates service user `phishlab`
- Installs Go 1.22.3
- Builds `evilginx2 v3.3.0` into `/opt/evilginx2` (binary granted
  `cap_net_bind_service` so it can listen on 80/443)
- Installs Gophish v0.12.1 into `/opt/gophish` (admin bound `127.0.0.1:8800`)
- Installs Mailhog (UI + SMTP both bound `127.0.0.1`)
- Builds `evilginx-lab` CLI to `/usr/local/bin/evilginx-lab`
- Writes systemd units for `evilginx`, `gophish`, `mailhog`, `evilginx-lab`
- Configures UFW (22/80/443 open; nothing else)
- Captures Gophish initial password from `journalctl`

### Prerequisites that must be true before `install.sh`

| What | How |
|---|---|
| SSH access for `deploy` user | Public key in `/home/deploy/.ssh/authorized_keys`, perms `600` |
| Passwordless sudo for `deploy` | One-line file at `/etc/sudoers.d/90-deploy`: `deploy ALL=(ALL) NOPASSWD: ALL` (chmod `440`) |
| Domain A record points at the host | See section 2 |
| Outbound 80/443 reachable | Required for cert issuance and package downloads |

### Engagement config (`evilginx-lab.yaml`)

Before the first `evilginx-lab init`, edit:

| Field | Set to |
|---|---|
| `engagement.start_date / end_date` | Current engagement window |
| `domain.phishing` | `login.cyb3rdefence.com` (or chosen subdomain) |
| `evilginx.install_dir` | `/opt/evilginx2` (NOT `/tmp/...`) |
| `store.path` | `/var/lib/evilginx-lab/evilginx-lab.db` |
| `gophish.api_key` | Empty initially; fill in after Gophish first login |

## 4. Engagement workflow

```bash
# On the phishlab host:
evilginx-lab init   -c evilginx-lab.yaml   # writes Evilginx + Gophish state
evilginx-lab deploy -c evilginx-lab.yaml   # starts services + dashboard
evilginx-lab status -c evilginx-lab.yaml   # engagement summary

# Operator browser (via SSH tunnel — see section 1):
#   http://localhost:8800   Gophish admin
#   http://localhost:8443   Evilginx-Lab dashboard
#   http://localhost:8025   Mailhog UI (intercepted test mail)
```

## 5. Verification

After `install.sh` finishes and `evilginx-lab deploy` runs:

```bash
# All services active?
for s in evilginx gophish mailhog evilginx-lab; do
  printf '%-15s %s\n' "$s" "$(sudo systemctl is-active "$s")"
done

# Public listeners on 80 / 443?
sudo ss -tlnp | grep -E ':(80|443) '

# Cert provisioned?
curl -sI "https://login.cyb3rdefence.com/" | head -2
```

## 6. Rollback

| Concern | Action |
|---|---|
| Bad DNS change | Re-PUT records from the IONOS snapshot at `/tmp/cyb3rdefence-zone.json`, or use IONOS history (kept ~7 days) |
| Lab needs reinstall | `sudo rm -rf /opt/evilginx2 /opt/gophish /var/lib/evilginx-lab /etc/systemd/system/{evilginx,gophish,mailhog,evilginx-lab}.service && sudo systemctl daemon-reload` then re-run `install.sh` |
| Single service won't start | `sudo journalctl -u <service> -n 100` to read the failure cause; restart with `sudo systemctl restart <service>` |

## 7. Open follow-ups

- IPv6 AAAA records for the phishing subdomains (host has `2a01:4f8:c014:fa58::1`)
- Move Gophish API key into `evilginx-lab.yaml` after first login
- Disable SSH password auth on the host once key auth confirmed
