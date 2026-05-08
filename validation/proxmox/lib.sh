# lib.sh — shared helpers for the Proxmox validation orchestrator.
#
# Sourced by every script in this directory. Defines:
#   px_init        load .env (PROXMOX_API_URL, PROXMOX_API_TOKEN), check curl/jq
#   px_api METHOD PATH [curl-args...]   HTTPS request with auth + cert skip
#   px_task TASK_UPID   block until a Proxmox task completes
#   px_next_vmid   first free VMID in the 9100-9199 range
#   px_ip VMID     wait for QEMU agent to report a non-loopback IPv4
#   px_sshkey_b64  pre-URL-encoded SSH pubkey (Proxmox quirk)
#   px_ssh VMID|IP CMD   SSH into a clone, refreshing host key as needed
#
# All scripts must:
#   set -euo pipefail
#   source "$(dirname "$0")/lib.sh"
#   px_init

set -uo pipefail

# Colors (only when stderr is a tty).
if [ -t 2 ]; then
    PX_BOLD=$'\033[1m'; PX_DIM=$'\033[2m'; PX_RED=$'\033[31m'; PX_GREEN=$'\033[32m'; PX_RESET=$'\033[0m'
else
    PX_BOLD=''; PX_DIM=''; PX_RED=''; PX_GREEN=''; PX_RESET=''
fi

px_log()  { printf '%s[proxmox]%s %s\n' "$PX_DIM" "$PX_RESET" "$*" >&2; }
px_ok()   { printf '%s[ok]%s %s\n' "$PX_GREEN" "$PX_RESET" "$*" >&2; }
px_fail() { printf '%s[fail]%s %s\n' "$PX_RED" "$PX_RESET" "$*" >&2; exit 1; }

# px_init — load env from repo-root .env, ensure required tools, set
# global PROXMOX_API_URL_BASE (no trailing slash).
px_init() {
    local repo_root
    repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
    local env_file="$repo_root/.env"
    [ -f "$env_file" ] || px_fail "missing $env_file (PROXMOX_API_URL, PROXMOX_API_TOKEN)"
    set -a; . "$env_file"; set +a
    : "${PROXMOX_API_URL:?not set}"
    : "${PROXMOX_API_TOKEN:?not set}"
    PROXMOX_API_URL_BASE="${PROXMOX_API_URL%/}/api2/json"
    PROXMOX_NODE="${PROXMOX_NODE:-srv}"
    PROXMOX_TEMPLATE_VMID="${PROXMOX_TEMPLATE_VMID:-9100}"
    PROXMOX_VMID_RANGE_LO="${PROXMOX_VMID_RANGE_LO:-9110}"
    PROXMOX_VMID_RANGE_HI="${PROXMOX_VMID_RANGE_HI:-9199}"
    for cmd in curl jq python3 ssh; do
        command -v "$cmd" >/dev/null 2>&1 || px_fail "$cmd not on PATH"
    done
    export PROXMOX_API_URL_BASE PROXMOX_NODE PROXMOX_TEMPLATE_VMID \
        PROXMOX_VMID_RANGE_LO PROXMOX_VMID_RANGE_HI
}

# px_api METHOD PATH [extra-curl-args...] — print response JSON to
# stdout, exit non-zero on HTTP >= 400.
px_api() {
    local method="$1" path="$2"; shift 2
    local body http
    body=$(curl -ksS -H "Authorization: PVEAPIToken=$PROXMOX_API_TOKEN" \
        -X "$method" "${PROXMOX_API_URL_BASE}${path}" \
        -w '\n__HTTP_STATUS__%{http_code}' "$@" 2>&1) || \
        px_fail "curl failed: $body"
    http="${body##*__HTTP_STATUS__}"
    body="${body%__HTTP_STATUS__*}"
    if [ "$http" -ge 400 ]; then
        px_fail "HTTP $http on $method $path: $body"
    fi
    printf '%s' "$body"
}

# px_task UPID — poll task status until it finishes; non-zero if
# the task ended with a non-OK status.
px_task() {
    local upid="$1" status
    for _ in $(seq 1 90); do
        status=$(px_api GET "/nodes/$PROXMOX_NODE/tasks/$upid/status" \
            | jq -r '.data.status // "?"')
        case "$status" in
            stopped) break ;;
            running) sleep 2 ;;
            *)       sleep 2 ;;
        esac
    done
    local exit_status
    exit_status=$(px_api GET "/nodes/$PROXMOX_NODE/tasks/$upid/status" \
        | jq -r '.data.exitstatus // "?"')
    [ "$exit_status" = "OK" ] || px_fail "task $upid ended with $exit_status"
}

# px_next_vmid — first integer in [LO, HI] not currently used.
px_next_vmid() {
    local in_use
    in_use=$(px_api GET "/cluster/resources?type=vm" \
        | jq -r '.data[].vmid')
    local v
    for v in $(seq "$PROXMOX_VMID_RANGE_LO" "$PROXMOX_VMID_RANGE_HI"); do
        grep -qx "$v" <<<"$in_use" || { echo "$v"; return; }
    done
    px_fail "no free VMID in $PROXMOX_VMID_RANGE_LO-$PROXMOX_VMID_RANGE_HI"
}

# px_ip VMID — wait up to ~3 min for the QEMU agent to report a
# non-loopback IPv4 address; print the address.
px_ip() {
    local vmid="$1" ip=""
    for _ in $(seq 1 30); do
        ip=$(px_api GET "/nodes/$PROXMOX_NODE/qemu/$vmid/agent/network-get-interfaces" 2>/dev/null \
            | jq -r '.data.result[]? | .["ip-addresses"][]?
                | select(.["ip-address-type"]=="ipv4" and .["ip-address"] != "127.0.0.1")
                | .["ip-address"]' 2>/dev/null | head -1)
        [ -n "$ip" ] && { echo "$ip"; return; }
        sleep 6
    done
    px_fail "VM $vmid did not report an IPv4 within timeout"
}

# px_sshkey_b64 PUBKEY_FILE — emit the double-URL-encoded form
# Proxmox's API requires for the cloud-init sshkeys field.
px_sshkey_b64() {
    local f="$1"
    [ -f "$f" ] || px_fail "ssh key file $f missing"
    tr -d '\n' < "$f" | python3 -c '
import sys, urllib.parse
s = sys.stdin.read()
print(urllib.parse.quote(urllib.parse.quote(s, safe=""), safe=""))
'
}

# px_ssh HOST CMD — ssh into HOST (IP or VMID-resolved), accepting
# new host key. Returns SSH exit status.
px_ssh() {
    local host="$1"; shift
    # Wipe any stale host key (clones share IPs from DHCP pool).
    ssh-keygen -R "$host" >/dev/null 2>&1 || true
    ssh -o StrictHostKeyChecking=accept-new \
        -o UserKnownHostsFile=/dev/null \
        -o LogLevel=ERROR \
        -i "${PX_SSH_KEY:-$HOME/.ssh/id_ed25519}" \
        "${PX_SSH_USER:-dani}@$host" "$@"
}
