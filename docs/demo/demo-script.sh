#!/usr/bin/env bash
# Simulated PhishRig CLI demo for GIF recording
# This script prints realistic output with typing delays

set -e

# Typing simulation
type_cmd() {
    local cmd="$1"
    printf '\033[1;32m$\033[0m '
    for ((i=0; i<${#cmd}; i++)); do
        printf '%s' "${cmd:$i:1}"
        sleep 0.04
    done
    echo
    sleep 0.3
}

pause() { sleep "${1:-1.2}"; }

clear
echo ""
printf '\033[1;36m  ____  _     _     _     ____  _       \033[0m\n'
printf '\033[1;36m |  _ \\| |__ (_)___| |__ |  _ \\(_) __ _ \033[0m\n'
printf '\033[1;36m | |_) |  _ \\| / __| _ \\| |_) | |/ _` |\033[0m\n'
printf '\033[1;36m |  __/| | | | \\__ \\ | | |  _ <| | (_| |\033[0m\n'
printf '\033[1;36m |_|   |_| |_|_|___/_| |_|_| \\_\\_|\\__, |\033[0m\n'
printf '\033[1;36m                                   |___/ \033[0m\n'
echo ""
printf '\033[0;90m  Red Team Phishing Engagement Platform\033[0m\n'
echo ""
pause 2

# Step 1: Init
type_cmd "phishrig init ACME-2026Q1 -c phishrig.yaml"
pause 0.5
echo ""
printf '\033[1;33m[init]\033[0m Loading config from phishrig.yaml...\n'
pause 0.3
printf '\033[1;33m[init]\033[0m Engagement: Q1 Red Team Assessment\n'
printf '\033[1;33m[init]\033[0m Client:     Acme Corp\n'
printf '\033[1;33m[init]\033[0m Domain:     login.acme-sso.com\n'
printf '\033[1;33m[init]\033[0m Phishlet:   o365\n'
printf '\033[1;33m[init]\033[0m Window:     2026-03-01 to 2026-03-31\n'
pause 0.3
printf '\033[1;32m[init]\033[0m Engagement ACME-2026Q1 initialized.\n'
printf '\033[1;32m[init]\033[0m Run \033[1mphishrig deploy\033[0m to start services.\n'
echo ""
pause 2

# Step 2: Deploy
type_cmd "phishrig deploy -c phishrig.yaml"
pause 0.5
echo ""
printf '\033[1;33m[deploy]\033[0m Restarting services...\n'
pause 0.4
printf '\033[1;32m  [+]\033[0m evilginx:  \033[1;32mactive\033[0m\n'
pause 0.2
printf '\033[1;32m  [+]\033[0m gophish:   \033[1;32mactive\033[0m\n'
pause 0.2
printf '\033[1;32m  [+]\033[0m mailhog:   \033[1;32mactive\033[0m\n'
pause 0.3
printf '\033[1;33m[deploy]\033[0m Session poller started (interval: 5s)\n'
printf '\033[1;33m[deploy]\033[0m Dashboard listening on \033[1m127.0.0.1:8443\033[0m\n'
printf '\033[1;32m[deploy]\033[0m Waiting for captures...\n'
echo ""
pause 1
printf '\033[0;90m{"time":"2026-03-15T10:32:15Z","level":"INFO","msg":"[poller] new credential captured: session xK9mP2 -> o365"}\033[0m\n'
pause 0.8
printf '\033[0;90m{"time":"2026-03-15T10:45:03Z","level":"INFO","msg":"[poller] new credential captured: session aB3nQ7 -> o365"}\033[0m\n'
pause 0.8
printf '\033[0;90m{"time":"2026-03-15T11:12:44Z","level":"INFO","msg":"[poller] new credential captured: session rT5wL1 -> o365"}\033[0m\n'
echo ""
pause 2

# Step 3: Status
type_cmd "phishrig status -c phishrig.yaml"
pause 0.5
echo ""
printf '\033[1;36m=== PhishRig Status ===\033[0m\n\n'
printf 'Engagement: Q1 Red Team Assessment\n'
printf '  Client:   Acme Corp\n'
printf '  Domain:   login.acme-sso.com\n'
printf '  Phishlet: o365\n'
printf '  Window:   2026-03-01 to 2026-03-31\n'
printf '  Status:   \033[1;32mactive\033[0m\n'
printf '  Captures: \033[1;33m3\033[0m\n\n'
printf 'Services:\n'
printf '  \033[1;32m[+]\033[0m evilginx:  active\n'
printf '  \033[1;32m[+]\033[0m gophish:   active\n'
printf '  \033[1;32m[+]\033[0m mailhog:   active\n'
printf '  \033[1;32m[+]\033[0m phishrig:  active\n\n'
printf 'Phishlets: 11 available (aws, facebook, github, google, instagram,\n'
printf '           linkedin, microsoft-live, microsoft-o365-adfs, o365,\n'
printf '           okta, twitter)\n'
echo ""
pause 2.5

# Step 4: Complete
type_cmd "phishrig complete -c phishrig.yaml"
pause 0.5
echo ""
printf '\033[1;32m[complete]\033[0m Engagement '\''Q1 Red Team Assessment'\'' (ID: ACME-2026Q1) marked as completed.\n'
printf '\033[1;32m[complete]\033[0m Total credentials captured: \033[1;33m3\033[0m\n'
echo ""
pause 2

# Step 5: Report
type_cmd "phishrig report --format csv --output report.csv -c phishrig.yaml"
pause 0.5
echo ""
printf '\033[1;32m[report]\033[0m Exported 3 credentials to report.csv\n'
echo ""
pause 1

type_cmd "cat report.csv"
pause 0.3
printf 'captured_at,phishlet,username,password,source_ip\n'
printf '2026-03-15T10:32:15Z,o365,j.smith@acme.com,********,198.51.100.42\n'
printf '2026-03-15T10:45:03Z,o365,m.johnson@acme.com,********,198.51.100.87\n'
printf '2026-03-15T11:12:44Z,o365,k.williams@acme.com,********,203.0.113.15\n'
echo ""
pause 2

# Done
echo ""
printf '\033[1;36m  Done. Full engagement lifecycle in 5 commands.\033[0m\n'
printf '\033[0;90m  phishrig init → deploy → status → complete → report\033[0m\n'
echo ""
pause 3
