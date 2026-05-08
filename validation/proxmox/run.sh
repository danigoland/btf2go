#!/usr/bin/env bash
# run.sh — run the validation runner inside a clone, fetch report.
#
# Usage:
#   run.sh VMID [--branch BRANCH] [--tier TIER] [--out PATH]
#   run.sh --ip IP [--branch ...] [--tier ...] [--out ...]
#
# Defaults: --branch master  --tier all  --out ./reports/<vmid>-<ts>.md
#
# The script clones the repo (private — uses `gh auth token` from the
# host) into the clone, builds btf2go, runs the requested tier(s),
# then scp's the resulting report.md back.

set -euo pipefail
source "$(dirname "$0")/lib.sh"
px_init

vmid="" ip="" branch="master" tier="all" out=""
while [ $# -gt 0 ]; do
    case "$1" in
        --branch) branch="$2"; shift 2 ;;
        --tier)   tier="$2"; shift 2 ;;
        --out)    out="$2"; shift 2 ;;
        --ip)     ip="$2"; shift 2 ;;
        --help|-h) sed -n '3,16p' "$0"; exit 0 ;;
        -*)        px_fail "unknown flag: $1" ;;
        *)         vmid="$1"; shift ;;
    esac
done

if [ -z "$ip" ]; then
    [ -n "$vmid" ] || px_fail "need VMID or --ip"
    ip=$(px_ip "$vmid")
fi
[ -n "$vmid" ] || vmid="$(echo "$ip" | tr -d .)"  # synth label for filenames

# Acquire a token for the private repo. Honor an explicit one in
# .env first; fall back to the host's gh auth token.
token="${BTF2GO_GH_TOKEN:-$(gh auth token 2>/dev/null || true)}"
[ -n "$token" ] || px_fail "no GitHub token available (set BTF2GO_GH_TOKEN or run 'gh auth login')"

ts=$(date +%Y%m%d-%H%M%S)
out="${out:-reports/${vmid}-${ts}.md}"
mkdir -p "$(dirname "$out")"

px_log "running validation on $ip (branch=$branch, tier=$tier)"
px_ssh "$ip" bash -s <<EOSSH
set -euo pipefail
cd ~
if [ ! -d btf2go ]; then
    git clone -q "https://x-access-token:${token}@github.com/danigoland/btf2go.git"
fi
cd btf2go
git fetch -q origin
git checkout -q "$branch"
git pull -q --ff-only origin "$branch" || true
go build -o /tmp/btf2go ./cmd/btf2go
bash validation/refresh.sh > /tmp/refresh.log 2>&1 || \
    { echo "[refresh failed — see /tmp/refresh.log]"; tail -10 /tmp/refresh.log; }
cd validation/runner
BTF2GO_BIN=/tmp/btf2go go run . run --tier "$tier"
EOSSH

px_log "fetching report -> $out"
scp -q -o StrictHostKeyChecking=accept-new \
    -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR \
    -i "${PX_SSH_KEY:-$HOME/.ssh/id_ed25519}" \
    "${PX_SSH_USER:-dani}@$ip:btf2go/validation/report.md" "$out"
px_ok "report saved: $out"

# Print headline so the caller can see the result inline.
sed -n '1,12p' "$out"
