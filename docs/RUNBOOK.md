# Operator Runbook

End-to-end workflow for running a phishing engagement on PhishLab 3.0
— from a clean Ubuntu VPS to delivering the engagement report.

If you're the dashboard operator and you've never used PhishLab
before, read this top-to-bottom once. Each step has a "Done when…"
checkpoint so you know whether to keep going.

## Pre-engagement checklist

Before you provision anything, you need:

- A signed Rules of Engagement (RoE) authorizing phishing against the target tenant
- The phishing domain you'll send from (apex you control, e.g. `acme-secure.com`)
- A target list (emails ± first/last/position)
- Knowledge of the target tenant's auth flow:
  - Cloud-only M365 → `o365` phishlet
  - ADFS-federated → `microsoft-o365-adfs` phishlet
  - 3rd-party SSO (Okta / GoDaddy SSO / Ping) → custom phishlet (not bundled; ~half-day to author)
- A clean Ubuntu 22.04+ VPS with public IPv4 + DNS pointing the phishing apex at it
- A Google account with OAuth 2.0 Client ID set up (for dashboard login)

**Done when:** you can answer "what phishlet, what domain, what targets, who's the operator?"

## 1. Provision the host

```bash
# As root or with sudo
git clone https://github.com/AuvaLabs/PhishLab-3.0.git
cd PhishLab-3.0
sudo bash install.sh phish.<your-apex>.com --with-ops-panel
```

This runs ~5 minutes and produces:
- Evilginx 3.3.0 + Gophish + Mailhog as systemd services
- evilginx-lab dashboard on `127.0.0.1:9000`
- nginx ops panel on `:8443` for `gophish.<apex>`, `mailhog.<apex>`, `dashboard.<apex>`
- `/etc/evilginx-lab/oauth.env` template + `/etc/evilginx-lab/allowlist.json` (empty)
- Daily backup cron at 03:30 UTC
- LE certificate via autocert on first lure hit

**Done when:** `sudo systemctl is-active evilginx evilginx-lab gophish mailhog nginx` returns `active` for all five.

Alternative: see `docs/DOCKER.md` for the containerised path.

## 2. Wire Google OAuth

In Google Cloud Console (`https://console.cloud.google.com/apis/credentials`):
1. Create an **OAuth 2.0 Client ID**, type **Web application**.
2. Authorized redirect URI: `https://dashboard.<apex>:8443/auth/google/callback`
3. Save the **Client ID + Client Secret**.

On the host:
```bash
sudo nano /etc/evilginx-lab/oauth.env
# Fill in:
#   GOOGLE_OAUTH_CLIENT_ID=<from console>
#   GOOGLE_OAUTH_CLIENT_SECRET=<from console>
#   GOOGLE_OAUTH_ALLOWED_EMAILS=you@example.com   (or use ALLOWED_DOMAINS)
sudo systemctl restart evilginx-lab
```

Then remove the basic-auth fallback so OAuth is the sole gate:
```bash
sudo sed -i '/server_name dashboard\./,/^}/ {/auth_basic/d;/auth_basic_user_file/d}' \
  /etc/nginx/sites-available/phishlab-ops
sudo nginx -t && sudo systemctl reload nginx
```

**Done when:** `https://dashboard.<apex>:8443/` redirects you to Google sign-in, and after consent you land on the dashboard with your engagement card visible.

## 3. Set the engagement metadata

In the dashboard, top of the page:
1. Click **Edit / New** on the Engagement card.
2. Set the **Engagement ID** (e.g. `ENG-2026-002`), **Client**, **Operator**, **Window** (`YYYY-MM-DD`), **Domain**, **Phishlet**, **RoE** reference, **Notes**.
3. Click **Save**.

The DNS Health card next to it shows pass/warn/fail for MX, SPF, DKIM, DMARC of your phishing domain. Address any failures **before** you launch a campaign — without DKIM, M365 typically routes your mail to junk.

**Done when:** Engagement card shows the new ID + status `active`, DNS Health is all-green (or you've consciously accepted the risk).

## 4. Enable a phishlet + create a lure

Click the **Phishlets** tab.

1. Find your phishlet (e.g. `o365` for cloud-only M365), click **Enable**, and enter the hostname (your phishing apex, e.g. `acme-secure.com`).
2. Wait ~3s for evilginx to restart.
3. Click **+ Lure** on the same row. Leave the path blank for an auto-generated random path.

The lure URL appears immediately in the **Active Lures** card (e.g. `https://login.acme-secure.com/<random-path>`).

**Done when:** Active Lures shows your URL with a green Phishlet badge.

## 5. Build and launch the campaign

Click the **Campaigns** tab → **+ Launch Campaign**.

The form:
- **Campaign name** — leave blank for auto-generated; or set a meaningful one.
- **Recipients** — paste targets one per line. Format: `email,first,last,position` (only email is required).
- **Email template** — pick one from the Templates tab. If none exists, create one there first.
- **Phish URL** — auto-filled from the lure (or click **Use in Campaign** on the lure row).
- **Advanced** — sender profile + landing page (sane defaults selected automatically).

Click **Test Send** to fire one test email at a single address before committing the full campaign. Verify it lands in inbox (not junk) before launching.

Click **Launch & Send** when ready.

**Done when:** Campaigns table shows the new campaign with status `In progress` and per-target rows are queued in the Timeline.

## 6. Watch captures arrive

Click the **Credentials** tab. As victims click and authenticate, sessions appear with one of three badges:

| Badge | Meaning |
|---|---|
| **Visited** (gray) | Lure clicked, no creds entered |
| **Vulnerable** (amber) | Username/password POSTed |
| **Exploitable** (red, pulsing) | Microsoft session cookies stolen — replayable |

Click **Why?** on any row to expand the per-session recommendations (Entra ID admin actions, Conditional Access, token protection, etc.) tailored to the captured artifacts.

**Done when:** at least one session reaches **Exploitable** status (or the engagement window closes).

## 7. Validate CSOC detections (purple-team)

For each Exploitable session:

1. Click **Replay** on the row.
2. Confirm the prompt — this sends a live authenticated request from the dashboard host to `outlook.office.com` carrying the captured cookies.
3. The result modal shows whether the cookies authenticated, the redirect chain, and the source IP.
4. **Share the timestamp + source IP with the target's CSOC.** Have them check their SIEM for:
   - Sign-in from unfamiliar IP / atypical UA
   - Impossible-travel detection
   - Entra ID Identity Protection high-risk sign-in
   - Conditional Access policy enforcement
   - Token replay alerts (E5: Token Protection)

Record which alerts fired vs. didn't — this is the actual deliverable for a defensive engagement.

**Done when:** you have a list of "alerts that fired" and "alerts that should have fired but didn't" for each replayed session.

### What FIDO2 / passkeys / Windows Hello changes

If the target user authenticates with a **phishing-resistant factor** (FIDO2 security key, passkey, Windows Hello for Business, smart-card / certificate auth), here is what actually happens vs. what most operators expect:

| Capture stage | Result against phishing-resistant auth |
|---|---|
| User clicks lure URL | works (transport is just HTTP) |
| User lands at proxied login page | works (the proxy is rewriting Microsoft's HTML) |
| User completes WebAuthn challenge | **fails** — the FIDO2 protocol cryptographically binds the assertion to the **origin (`window.location.hostname`)** the browser sees. A passkey on `login.cyb3rdefence.com` will not produce an assertion that `login.microsoftonline.com` accepts |
| Post-auth session cookie capture | works **only if** the user falls back to a phishable factor (password + push, password + TOTP) before completing WebAuthn |

This is by design — FIDO2 is the canonical anti-phishing factor, and the binding is enforced by the browser, not Microsoft. No reverse-proxy AitM tool (evilginx, Modlishka, Muraena) can defeat it without exploiting an unrelated browser vulnerability.

**Practical RoE language to surface with the client:**

> Phishing-resistant authentication factors (FIDO2 security keys, passkeys, Windows Hello for Business, certificate-based auth) cannot be captured by this engagement's adversary-in-the-middle methodology. Users protected by these factors will be observed clicking the lure but **not** as Vulnerable or Exploitable. Successful captures against such targets indicate the user fell back to a phishable factor — itself a finding worth surfacing.

When you brief defenders, the corollary is: **enrolling privileged accounts in FIDO2 / passkey is the single largest reduction in AitM phishing risk.** The detection-validation work above is most useful for accounts that cannot be moved to FIDO2 (legacy services, shared mailboxes, contractor identities).

## 8. Generate the engagement report

Click **Report** on the Engagement card. This downloads a Markdown
report with:
- Engagement metadata
- Captured-sessions table with status + recommendations
- Campaigns + timeline
- Summary counts

Send to the client. The recommendations are vendor-specific
(`Revoke-MgUserSignInSession`, etc.) so they paste straight into the
client's remediation plan.

**Done when:** report delivered + signed off.

## 9. Wrap up

In the dashboard:
1. Click **Clear Data** on the Engagement card to wipe captures
   and timeline (engagement record preserved). Confirms with a
   destructive dialog.
2. (Optional) bump the Engagement ID via **Edit / New** for the
   next client; or disable phishlets via the Phishlets tab to take
   the lure offline immediately.

On the host:
- Captures live on disk in `/opt/evilginx2/state/data.db`. The
  dashboard's Clear Data does NOT touch this — to fully wipe,
  `sudo systemctl stop evilginx && sudo rm /opt/evilginx2/state/data.db && sudo systemctl start evilginx`.
- Backup snapshots at `/var/backups/phishlab/` are pruned to last
  14. Manual cleanup with `rm /var/backups/phishlab/*.tar.gz` if
  you want them gone.

**Done when:** target tenant has rotated affected credentials,
report delivered, captures wiped, lure disabled.

## Troubleshooting

| Symptom | Fix |
|---|---|
| Dashboard funnel chart blank | Hard-refresh browser (Ctrl-F5). If still blank, check DevTools console for Chart.js load errors. |
| Login redirect loops | Check `/etc/evilginx-lab/oauth.env` for correct redirect URI matching the Google Console value exactly (port + path). |
| Test email lands in junk | DNS Health card almost certainly shows missing DKIM. Activate DKIM in your DNS provider (publishes 2-3 CNAMEs); check the card again after ~10 min. |
| Lure URL returns rickroll instead of phishlet page | The URL was hit from a blacklisted IP. Check `sudo cat /opt/evilginx2/state/blacklist.txt`. Evilginx auto-blacklists scanners aggressively — operators must avoid hitting non-lure paths from their test IP. |
| Replay says "bounced to login" | Cookies expired. Capture is still useful as evidence; the live session is gone. |
| Service won't start after install | `sudo journalctl -u evilginx-lab -n 50 --no-pager`. If it complains about port 53, run `sudo bash docker/setup-host.sh` (works for non-Docker installs too — frees port 53 from systemd-resolved). |

## Reference

- [`DEPLOY.md`](DEPLOY.md) — install.sh internals
- [`USAGE.md`](USAGE.md) — engagement.yaml schema
- [`DOCKER.md`](DOCKER.md) — containerised deployment
- Upstream: <https://github.com/AuvaLabs/PhishLab-3.0>
