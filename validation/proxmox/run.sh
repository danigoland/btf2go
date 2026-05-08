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

vmid="" ip="" branch="master" out="" kernel=0; tiers=()
while [ $# -gt 0 ]; do
    case "$1" in
        --branch) branch="$2"; shift 2 ;;
        --tier)   tiers+=("$2"); shift 2 ;;
        --out)    out="$2"; shift 2 ;;
        --ip)     ip="$2"; shift 2 ;;
        --kernel) kernel=1; shift ;;
        --help|-h) sed -n '3,16p' "$0"; exit 0 ;;
        -*)        px_fail "unknown flag: $1" ;;
        *)         vmid="$1"; shift ;;
    esac
done
[ ${#tiers[@]} -gt 0 ] || tiers=(all)
# Build the runner --tier args; "all" stays a single value.
runner_tier_args=()
for t in "${tiers[@]}"; do runner_tier_args+=(--tier "$t"); done
[ "$kernel" = 1 ] && runner_tier_args+=(--kernel)
tier_label=$(IFS=,; echo "${tiers[*]}")

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

px_log "running validation on $ip (branch=$branch, tiers=$tier_label)"
# Build the remote command line; quote each --tier arg defensively.
runner_args_q=""
for a in "${runner_tier_args[@]}"; do
    runner_args_q+=" $(printf '%q' "$a")"
done
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
go build -o /tmp/validation-runner .
sudo_pfx=""
if [ "$kernel" = 1 ]; then sudo_pfx="sudo -E"; fi
BTF2GO_BIN=/tmp/btf2go \$sudo_pfx /tmp/validation-runner run$runner_args_q
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
