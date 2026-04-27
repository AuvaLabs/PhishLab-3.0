<p align="center">
  <img src="docs/landing/assets/logo.svg" alt="PhishLab 3.0" width="360"/>
</p>

<p align="center">
  <strong>Adversary-in-the-middle phishing engagement platform with same-day defender-side detection validation.</strong>
</p>

<p align="center">
  <a href="https://auvalabs.github.io/PhishLab-3.0/landing/">Public landing</a> &middot;
  <a href="docs/RUNBOOK.md">Operator runbook</a> &middot;
  <a href="docs/DEPLOY.md">Deploy guide</a> &middot;
  <a href="docs/DOCKER.md">Docker</a> &middot;
  <a href="docs/USAGE.md">Engagement workflow</a>
</p>

> [!CAUTION]
> **For authorized testing only.** Use of this platform against systems you do not own or have explicit written permission (signed Rules of Engagement) to test is illegal in most jurisdictions and is not supported by the maintainers.

---

## What's Included

- **Evilginx3 v3.3.0** &mdash; reverse-proxy phishing framework with MFA-relay and session-cookie capture
- **Gophish v0.12.1** &mdash; campaign management + email delivery + tracking
- **Mailhog** &mdash; local SMTP capture for development and dry-runs
- **evilginx-lab dashboard** &mdash; Go service with Google OAuth login that orchestrates all of the above:
  - Engagement metadata, captures, status badges (Visited / Vulnerable / Exploitable), per-session vendor-specific recommendations
  - One-form campaign launch with paste-emails recipient list
  - Active Lures card with Copy + "Use in Campaign"
  - DNS Health card (MX / SPF / DKIM / DMARC pass-fail-warn)
  - Phishlet management (enable / disable / create lure) &mdash; no SSH required
  - Audit log of every state-changing action (forensic trail for multi-operator engagements)
  - User allowlist management (Google OAuth, runtime add/remove via UI)
  - Capture replay against `outlook.office.com` for purple-team detection validation
  - Markdown engagement-report export ready for the client deliverable
  - Real-time capture notifications: WebAudio chirp + browser-tab title flash + toast
  - Cookie-Editor JSON export per session
- **Systemd services** + **UFW firewall** + **nginx ops panel** with TLS
- **Daily backup cron** to `/var/backups/phishlab/` with 14-day retention
- **One-shot bare-metal install OR Docker compose stack**

---

## Requirements

- Ubuntu 22.04+ x64 VPS (clean), root SSH access
- Public IPv4 with DNS A-records pointing the phishing apex at it
- Ports 53/UDP+TCP, 80, 443 open to the public internet (53 is for evilginx autocert + phishlet DNS)
- For the Docker path: Docker Engine 24+ + `docker compose` plugin

---

## Installation

### Bare metal (recommended for production)

```bash
git clone https://github.com/AuvaLabs/PhishLab-3.0.git
cd PhishLab-3.0
sudo bash install.sh phish.yourdomain.com --with-ops-panel
```

`install.sh` is idempotent &mdash; re-run safely. The `--with-ops-panel` flag provisions an nginx reverse-proxy on port 8443 with a self-signed cert so the dashboard, Gophish, and Mailhog are reachable from a workstation without an SSH tunnel.

### Docker

```bash
git clone https://github.com/AuvaLabs/PhishLab-3.0.git
cd PhishLab-3.0
sudo bash docker/setup-host.sh        # one-time host prep (frees :53 from systemd-resolved)
docker compose up -d --build
```

See [`docs/DOCKER.md`](docs/DOCKER.md) for known limitations (Linux-only for evilginx, no automatic LE renewal in container yet).

### Post-install (5 minutes)

1. Configure Google OAuth: edit `/etc/evilginx-lab/oauth.env` with your OAuth Client ID + Secret + allowlisted email
2. Restart the dashboard: `sudo systemctl restart evilginx-lab`
3. Remove `auth_basic` from `/etc/nginx/sites-available/phishlab-ops` for the dashboard vhost so OAuth becomes the sole gate
4. Open `https://dashboard.your-apex.com:8443/` &mdash; sign in with Google

The full step-by-step lives in [`docs/RUNBOOK.md`](docs/RUNBOOK.md) &mdash; fresh-VPS to client-deliverable in nine numbered steps with a troubleshooting table.

---

## Post-Install Setup

### 1. Configure Evilginx3

Evilginx3 v3.3.0 stores its state in the `-c` directory (default `/opt/evilginx2/state`). To configure interactively the first time, stop the service and run the binary against the same state dir:

```bash
systemctl stop evilginx
sudo HOME=/opt/evilginx2/state /opt/evilginx2/dist/evilginx \
  -c /opt/evilginx2/state -p /opt/evilginx2/phishlets
```

Inside the evilginx prompt, paste the commands from `/root/evilginx_setup_commands.txt` (v3 syntax):

```
config domain yourdomain.com
config ipv4 external <YOUR_IP>
config autocert on
phishlets hostname o365 yourdomain.com
phishlets enable o365
```

Once configured (autocert will request Let's Encrypt certificates over port 80), exit cleanly and restart the service:

```bash
systemctl start evilginx
```

Then create a lure to get the entry URL: `lures create o365` then `lures get-url 0`.

### 2. Access Gophish Admin

Gophish admin is bound to `127.0.0.1` for security. Access it via SSH tunnel:

```bash
ssh -L 8800:127.0.0.1:8800 root@YOUR_SERVER_IP
```

Then open `http://localhost:8800` in your browser.

The initial admin password is printed in the Gophish service log:

```bash
journalctl -u gophish | grep password
```

### 3. Connect Gophish to Mailhog

1. Open the Gophish admin UI
2. Navigate to **Sending Profiles**
3. Create a new profile with SMTP host: `localhost:1025`
4. No authentication required
5. Send a test email - it will appear in the Mailhog UI

### 4. View Captured Emails

Mailhog UI is bound to `127.0.0.1` for security. Access via SSH tunnel:

```bash
ssh -L 8025:127.0.0.1:8025 deploy@YOUR_SERVER_IP
```

Then open `http://localhost:8025`.

---

## Services

| Service   | Command                        | Port(s)          |
|-----------|--------------------------------|------------------|
| Evilginx  | `systemctl status evilginx`    | 80, 443          |
| Gophish   | `systemctl status gophish`     | 8800 (localhost)  |
| Mailhog   | `systemctl status mailhog`     | 8025 (UI), 1025 (SMTP, localhost) |

Manage services with:

```bash
systemctl start|stop|restart|status <service>
journalctl -u <service> -f    # follow logs
```

---

## Firewall Rules

| Port | Exposure | Service                         |
|------|----------|---------------------------------|
| 22   | public   | SSH                             |
| 80   | public   | HTTP (Evilginx, autocert)       |
| 443  | public   | HTTPS (Evilginx phishing proxy) |
| 8800 | loopback | Gophish admin (SSH tunnel)      |
| 8081 | loopback | Gophish phish (tracking pixel)  |
| 8025 | loopback | Mailhog UI (SSH tunnel)         |
| 1025 | loopback | Mailhog SMTP                    |
| 8443 | loopback | Evilginx-Lab dashboard          |

Only `22/80/443` are opened in UFW; everything else is reached via SSH tunnel.

---

## Included Phishlets

The `phishlets/` directory contains 11 pre-configured phishlets ready for use with Evilginx3 v3.3.0.

### Native Evilginx3 (min_ver 3.0.0+)

| Phishlet | Target | Key Tokens |
|----------|--------|------------|
| `microsoft-live.yaml` | login.live.com | SDIDC, JSHP |
| `microsoft-o365-adfs.yaml` | login.microsoftonline.com + ADFS | ESTSAUTH, ESTSAUTHPERSISTENT |
| `okta.yaml` | Okta tenants (template) | idx |
| `twitter.yaml` | twitter.com / X | kdt, auth_token, ct0, twid |
| `linkedin.yaml` | linkedin.com (with evilpuppet) | li_at |

### Evilginx2-Compatible (work in v3 via backward compat)

| Phishlet | Target | Key Tokens |
|----------|--------|------------|
| `o365.yaml` | login.microsoftonline.com | ESTSAUTH, ESTSAUTHPERSISTENT |
| `google.yaml` | accounts.google.com | SID, HSID, SSID, GAPS |
| `github.yaml` | github.com | user_session, _gh_sess |
| `facebook.yaml` | facebook.com | c_user, xs, sb |
| `instagram.yaml` | instagram.com | sessionid |
| `aws.yaml` | signin.aws.amazon.com | aws-creds, JSESSIONID |

### Notes

- **Okta** requires replacing `<okta-tenant-placeholder>` with your target's tenant name
- **O365 ADFS** requires replacing `example.com` with the actual ADFS domain

---

## Security Notes

- Gophish admin is bound to `127.0.0.1` - always access via SSH tunnel
- Mailhog SMTP is bound to localhost - only accessible from the server itself
- Services run under a dedicated `phishlab` user, not root
- Change the default Gophish password immediately after first login
- This lab is intended for **authorized security testing only**

---

## License

MIT
