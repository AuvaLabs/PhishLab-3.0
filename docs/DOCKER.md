# Docker deployment

PhishLab 3.0 ships with `Dockerfile` + `docker-compose.yml` for
containerised deployment. This is an alternative to `bash install.sh` —
the same operator dashboard, OAuth, captures pipeline, but in
containers.

## When to use Docker

| Scenario | Recommended |
|---|---|
| Production engagement on a dedicated VPS | **`bash install.sh`** (bare metal) |
| Dev / lab / quick test | **Docker** |
| Multiple parallel engagements isolated by container | **Docker** |
| Existing host already runs other services on :53 / :80 / :443 | Neither — get a clean VPS |

## Prerequisites

- Docker Engine 24+ and the `docker compose` plugin (V2)
- Linux host (the `network_mode: host` for evilginx assumes Linux; macOS/Windows Docker Desktop do not work for evilginx itself)
- Public IPv4 with DNS for the engagement domain pointed at the host
- Root on the host for one-time `:53` cleanup

## One-time host prep

Evilginx needs `:53/tcp+udp` for autocert + phishlet DNS. On Ubuntu this
collides with `systemd-resolved`. Run the bundled helper once:

```bash
sudo bash docker/setup-host.sh
```

This stops `systemd-resolved`, replaces `/etc/resolv.conf` with static
upstreams (1.1.1.1, 8.8.8.8), and verifies `:53` is free.

## Bring up the stack

```bash
# 1. Engagement config
cp configs/engagement.example.yaml evilginx-lab.yaml
# edit evilginx-lab.yaml with your engagement id, client, dates, RoE

# 2. OAuth config (optional — leave empty for nginx-basic-auth fallback)
sudo mkdir -p /etc/evilginx-lab
sudo bash -c 'cat > /etc/evilginx-lab/oauth.env <<EOF
GOOGLE_OAUTH_CLIENT_ID=
GOOGLE_OAUTH_CLIENT_SECRET=
GOOGLE_OAUTH_REDIRECT_URL=https://dashboard.your-apex.com:8443/auth/google/callback
GOOGLE_OAUTH_ALLOWED_EMAILS=you@example.com
GOOGLE_OAUTH_ALLOWED_DOMAINS=
SESSION_COOKIE_SECRET=$(openssl rand -hex 32)
OAUTH_ALLOWLIST_FILE=/etc/evilginx-lab/allowlist.json
EOF'
sudo chmod 0600 /etc/evilginx-lab/oauth.env

# 3. Build + start
docker compose up -d --build

# 4. Watch the dashboard
docker compose logs -f evilginx-lab
```

## Service map

| Container | Image | Network | Public ports |
|---|---|---|---|
| `phishlab-evilginx` | `kgretzky/evilginx2:3.3.0` | host | 53/udp, 53/tcp, 80, 443 |
| `phishlab-gophish` | `gophish/gophish:latest` | bridge | 127.0.0.1:8800 (admin), 127.0.0.1:8081 (phish) |
| `phishlab-mailhog` | `mailhog/mailhog:latest` | bridge | 127.0.0.1:8025 (UI), 127.0.0.1:1025 (SMTP) |
| `phishlab-dashboard` | `auvalabs/phishlab-dashboard:dev` (built locally) | bridge | 127.0.0.1:9000 |

## Persistent volumes

- `evilginx-state` — `/opt/evilginx2/state` inside the evilginx container; mounted read-only into the dashboard so the poller can read `data.db`
- `gophish-data` — Gophish SQLite database
- `lab-db` — `evilginx-lab.db` (engagements, captures, timeline, campaigns)
- `oauth-config` — `/etc/evilginx-lab/` (oauth.env + allowlist.json)

To inspect or back up:
```bash
docker volume ls
docker run --rm -v phishlab_lab-db:/data alpine tar czf - /data > lab-db-backup.tgz
```

## Putting nginx + OAuth in front

The compose file exposes the dashboard on `127.0.0.1:9000` only —
not the public internet. To expose it via TLS at
`https://dashboard.your-domain:8443/`, run nginx (containerised or
bare-metal) in front. The bare-metal install.sh ops-panel block has
the right vhost template; copy that file into a sidecar nginx
container if you want the whole thing dockerised.

## Known limitations

- **Linux only** for evilginx. The `network_mode: host` directive
  doesn't behave the same way on macOS/Windows Docker Desktop, and
  the phishlet flow needs raw socket access.
- **No automatic LE renewal in the container yet** — autocert works
  on first run because the container can write to the
  `evilginx-state` volume, but rotation is the same flow as bare
  metal.
- **Dashboard cannot exec into evilginx CLI** for phishlet/lure
  management. The HTTP API endpoints (`/api/phishlets/enable`, etc.)
  edit `/opt/evilginx2/state/config.json` directly via the shared
  volume; the evilginx container is restarted via Docker (compose
  restart policy) rather than `systemctl`. **TODO**: a small
  sidecar that listens on a Unix socket and forwards `restart`
  signals to evilginx — current build calls `systemctl` which is
  a no-op inside containers.

## Switching back to bare metal

If you've been running Docker and want to switch:
```bash
docker compose down
# bare-metal install.sh expects port 53 free, which setup-host.sh
# already arranged, so no further host prep needed.
sudo bash install.sh phish.your-apex.com --with-ops-panel
```

The volumes from Docker remain on disk; the bare-metal install
ignores them. To clean up:
```bash
docker volume rm phishlab_evilginx-state phishlab_gophish-data phishlab_lab-db phishlab_oauth-config
```
