# Command Center Handoff — `cyb3rdefence.com` engagement

> **For**: command-center operator agent that will own day-to-day operation
> of the cyb3rdefence engagement going forward.
> **Snapshot date**: 2026-04-26.
> **Source of truth**: this file. Copy it into the command-center repo and
> keep it current as the engagement evolves.

---

## 1. What `cyb3rdefence` is

A Red Team engagement domain (`cyb3rdefence.com`) used to host two
independent capabilities for the AuvaLabs platform:

| Capability | Project | Repo |
|---|---|---|
| Phishing lab — Evilginx3 + Gophish + Mailhog + dashboard | **PhishLab 3.0** | `github.com/AuvaLabs/PhishLab-3.0` |
| Out-of-band callback receiver for security scanners | **redblue** (`perimetr` module) | `github.com/AuvaLabs/RedBlue-Perimetr` |

Both share the parent zone but live on **separate hosts** and have **no
runtime coupling**. They are tied together only by DNS records under
`cyb3rdefence.com`, all managed in one IONOS account.

---

## 2. Host inventory

| Role | Hostname | IPv4 | IPv6 | OS | Provider |
|---|---|---|---|---|---|
| **PhishLab host** | `cyb3rdefence-project` | `91.98.47.193` | `2a01:4f8:c014:fa58::1` | Ubuntu 24.04.3 LTS | Hetzner |
| **redblue / OOB host** ("this localhost") | `cyb3rsec` | `208.87.134.155` | n/a | Ubuntu 22.04 | (in-house) |
| **Command Center** | (ops workstation) | — | — | — | — |

SSH:
- **PhishLab**: `ssh deploy@91.98.47.193` (key auth; passwordless sudo via `/etc/sudoers.d/90-deploy`)
- **redblue**: local on `208.87.134.155`; no SSH-from-command-center configured yet — operator runs commands locally

---

## 3. Domain — `cyb3rdefence.com`

**Registrar / DNS provider**: IONOS.
**Zone ID**: `7634f821-1077-11ef-acfb-0a5864440c92`.
**API endpoint**: `https://api.hosting.ionos.com/dns/v1`.
**API credentials location** (operator-side, NEVER in git):

```
~/.config/cyb3rdefence/ionos.env       # mode 0600
  IONOS_PUBLIC_PREFIX=...
  IONOS_SECRET=...
  IONOS_API_KEY="${IONOS_PUBLIC_PREFIX}.${IONOS_SECRET}"
```

Header for direct API calls: `X-API-Key: $IONOS_API_KEY`.

### Records that matter

| Hostname | Type | Target | Owner | Purpose |
|---|---|---|---|---|
| `cyb3rdefence.com` | A | `91.98.47.193` | PhishLab | apex |
| `login.cyb3rdefence.com` | CNAME | apex | PhishLab | phishing landing |
| `phish.cyb3rdefence.com` | A | `91.98.47.193` | PhishLab | phishing landing |
| `securephish.cyb3rdefence.com` | A | `91.98.47.193` | PhishLab | phishing landing |
| `securephish01.cyb3rdefence.com` | A | `91.98.47.193` | PhishLab | phishing landing |
| `securephish02.cyb3rdefence.com` | A | `91.98.47.193` | PhishLab | phishing landing |
| `oob.cyb3rdefence.com` | A | `208.87.134.155` | redblue | OOB callback apex |
| `ns1.oob.cyb3rdefence.com` | A | `208.87.134.155` | redblue | NS glue (auth NS for `*.oob`) |
| `ns2.oob.cyb3rdefence.com` | A | `208.87.134.155` | redblue | NS glue |
| `oob.cyb3rdefence.com` | NS | `ns{1,2}.oob.cyb3rdefence.com` | redblue | delegate `*.oob` subtree to redblue's interactsh |

**Operator rule**: never repoint `oob.*` records without coordinating
with the redblue operator. They are the OOB chain that perimetr's
scanners (nuclei / ysoserial / phpggc) depend on for blind-class
verification.

Spot-check the whole zone:

```bash
set -a; source ~/.config/cyb3rdefence/ionos.env; set +a
curl -sS -H "X-API-Key: $IONOS_API_KEY" \
  "https://api.hosting.ionos.com/dns/v1/zones/7634f821-1077-11ef-acfb-0a5864440c92" \
  | jq -r '.records[] | "\(.type)\t\(.name)\t\(.content)"' | sort
```

---

## 4. PhishLab 3.0 stack (on `91.98.47.193`)

**Repo**: `github.com/AuvaLabs/PhishLab-3.0`
**Local checkout (operator workstation)**: `/home/deploy/phishlab/Evilginx3PhishLab` (folder kept as-is for now)
**Remote install path**: `/home/deploy/Evilginx3PhishLab`
**Binary**: `/usr/local/bin/evilginx-lab` (CLI: `init / deploy / status / serve`)

### Components

| Service | Bind | Public? | Notes |
|---|---|---|---|
| `evilginx.service` | `0.0.0.0:80,443` | yes | Reverse-proxy phishing on the configured phishlet hostname |
| `gophish.service` admin | `127.0.0.1:8800` | no | SSH tunnel only |
| `gophish.service` phish (tracking) | `127.0.0.1:8081` | no | non-default; default `0.0.0.0:80` collides with Evilginx |
| `mailhog.service` UI/API | `127.0.0.1:8025` | no | SSH tunnel only |
| `mailhog.service` SMTP | `127.0.0.1:1025` | no | Gophish sending profile target |
| `evilginx-lab.service` dashboard | `127.0.0.1:8443` | no | SSH tunnel only — started by `evilginx-lab deploy` |
| UFW | open: `22/80/443` only | — | everything else loopback |

### Operator tunnel (one-liner from the command center)

```bash
ssh -L 8800:127.0.0.1:8800 \
    -L 8025:127.0.0.1:8025 \
    -L 8443:127.0.0.1:8443 \
    deploy@91.98.47.193
```

Then: `http://localhost:8800` (Gophish), `http://localhost:8025` (Mailhog), `http://localhost:8443` (Evilginx-Lab).

### Engagement cycle

1. Edit `evilginx-lab.yaml` on the host (set engagement window, phishlet, targets, SMTP, gophish API key)
2. `evilginx-lab init   -c evilginx-lab.yaml` — writes Evilginx + Gophish state
3. `evilginx-lab deploy -c evilginx-lab.yaml` — starts services + dashboard
4. `evilginx-lab status -c evilginx-lab.yaml` — engagement summary
5. `evilginx-lab report --format csv -o engagement-report.csv` — final export

For the bring-up runbook see [`DEPLOY.md`](DEPLOY.md). For the engagement
workflow see [`USAGE.md`](USAGE.md).

### Known errors and their fixes (carried in install.sh — do not regress)

| Symptom | Root cause | Where it's fixed |
|---|---|---|
| `go build` fails: `stat /opt/evilginx2/cmd/evilginx-lab: directory not found` | `SCRIPT_DIR` resolved after `cd /opt`, so dirname's relative path resolves wrong | `install.sh:13-16` — resolved at top before any `cd` |
| `PUBLIC_IP` resolves to IPv6 on dual-stack | `curl ifconfig.me` defaulted to v6 | `install.sh:43` — forced `curl -4` |
| Mailhog UI publicly browsable on `:8025` | bind was `0.0.0.0:8025` | `install.sh:222-224` — bound `127.0.0.1` |
| Gophish stuck restarting: `bind: permission denied 0.0.0.0:80` | `phish_server` default collides with Evilginx + needs root | `install.sh:156-163` — bound `127.0.0.1:8081` |
| Evilginx exits cleanly status=0 under systemd | TUI readline EOFs on null stdin | `install.sh:203` — `ExecStart=/bin/bash -c 'sleep infinity \| evilginx ...'` |

---

## 5. redblue OOB stack (on `208.87.134.155`)

**Repo**: `github.com/AuvaLabs/RedBlue-Perimetr` (and the parent `RedBlue` repo for the docker-compose stack)
**Local path**: `/home/deploy/redblue`
**Compose file**: `/home/deploy/redblue/docker-compose.yml`
**OOB-specific runbook**: `/home/deploy/redblue/perimetr/docs/runbooks/oob-infrastructure.md`

### Components (containerized)

| Container | Image | Bind |
|---|---|---|
| `redblue-interactsh-1` | `projectdiscovery/interactsh-server:latest` | `208.87.134.155:53/udp+tcp` (DNS); `127.0.0.1:8881-8882` (HTTP/HTTPS callback) |
| `redblue-perimetr-app-1` | local | `127.0.0.1:13000` |
| (~15 other redblue services) | — | mostly loopback |

`oob.cyb3rdefence.com:80/443` is fronted by an nginx vhost
(`/home/deploy/redblue/deploy/nginx/oob.cyb3rdefence.com`) that proxies to
the interactsh container's `8881/8882`.

Auth token for interactsh-client connections: env var
`INTERACTSH_TOKEN` in the redblue compose; rotated by the redblue
operator. Command-center jobs that need to *use* the OOB receiver should
read the token from the redblue host and pass `-token` to
`interactsh-client`.

### Operator runbook for OOB

Full failure-mode table and operator commands live in
`/home/deploy/redblue/perimetr/docs/runbooks/oob-infrastructure.md`.
Quick checks from the command center:

```bash
# End-to-end resolution (any public resolver)
dig +short A "$(uuidgen | tr -d -).oob.cyb3rdefence.com" @1.1.1.1
# => 208.87.134.155

# IONOS referral correctness (glue records present?)
dig +norec @ns.ui-global-dns.com test.oob.cyb3rdefence.com A
# AUTHORITY should list ns1.oob/ns2.oob; ADDITIONAL must list 208.87.134.155
```

---

## 6. Secrets inventory

| Secret | Where it lives | Format | Used by |
|---|---|---|---|
| IONOS DNS API key | operator workstation `~/.config/cyb3rdefence/ionos.env` (0600) | `prefix.secret` | DNS automation, certbot DNS-01 |
| Gophish admin password | first set at install (printed once in `journalctl -u gophish`); changed by operator on first login | string | Gophish admin UI |
| Interactsh auth token | `redblue` container env `INTERACTSH_TOKEN` (compose `.env`) | random hex | interactsh-client runs in scanner pipelines |
| SSH public key authorised on phishlab host | `/home/deploy/.ssh/authorized_keys` on `91.98.47.193` | OpenSSH ed25519 | operator + command-center SSH |
| Operator/installation password (one-shot bootstrap) | rotated after first key login — **not retained** | — | original cloud-init bootstrap only |

**Rule**: nothing here goes into git. Everything lives outside the
checked-out repos. The command center's secret store should be the
authoritative copy of the IONOS key and any other shared secrets.

---

## 7. Recent change log (for context)

### 2026-04-26 evening — PhishLab move + repo rename

- Migrated PhishLab off `208.87.134.155` (where it had been co-located with redblue) onto a dedicated Hetzner VPS at `91.98.47.193`.
- Created A records for `phish` / `securephish` / `securephish01` / `securephish02` (previously had only mail records, no host record).
- During planning, `ns1.oob` / `ns2.oob` / `oob` A records were briefly repointed to `91.98.47.193` then **reverted** to `208.87.134.155` once it was confirmed redblue's interactsh stays in place.
- Cleaned up DNS residue: deleted `nw2.oob.*` (typo zone) and `www.{ns1,ns2,oob}.oob` records.
- Hardened `install.sh` (see error-fix table in §4).
- Removed duplicate sibling repo `PhishRig` (uncommitted state preserved at `/home/deploy/backups/phishrig-20260426-175645.tgz`); ported its `docs/`, `scripts/`, `.github/`, `engagement.yaml` example into PhishLab.
- Renamed GitHub repo `AuvaLabs/PhishRig` → `AuvaLabs/PhishLab-3.0` and force-pushed clean PhishLab content.

### Earlier (pre-snapshot)

The full DetectR / RedBlue platform history pre-dates this handoff. See
`/home/deploy/redblue/perimetr/docs/HANDOFF.md` and the assorted
auto-memory files in
`~/.claude/projects/-home-deploy-redblue/memory/` for that lineage.

---

## 8. Open follow-ups (non-blocking)

| Item | Owner | Notes |
|---|---|---|
| AAAA records for PhishLab subdomains | command center | Host already has IPv6 `2a01:4f8:c014:fa58::1` |
| Disable SSH password auth on phishlab host | command center | Key auth confirmed; flip `PasswordAuthentication no` in sshd_config |
| Move Gophish API key into `evilginx-lab.yaml` | first-login operator | Empty until UI login generates one |
| Phase 2 campaign orchestrator (`internal/campaign/`) | PhishLab maintainer | Code is on `main` as WIP; not on critical install path |
| README cleanup pass | PhishLab maintainer | Some pre-existing instructions (e.g., "phishlets enable microsoft") are stale; not a regression from this session |

---

## 9. How the command center should use this doc

1. Drop a copy at `<command-center-repo>/projects/cyb3rdefence.md` (or wherever the agent's project index lives).
2. On any DNS, host, secret, or repo change, update the relevant table here and re-sync.
3. When a new operator agent is brought online, it should read this file first as the engagement's authoritative summary, then drill into the per-project runbooks (`DEPLOY.md`, `USAGE.md`, `oob-infrastructure.md`).
4. This file is **safe to commit** to the command-center repo — it contains no secret values, only locations / shapes of secrets.
