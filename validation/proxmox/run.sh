#!/usr/bin/env bash
# run.sh — run the validation runner inside a clone, fetch report.
#
# Usage:
#   run.sh VMID [--branch BRANCH] [--tier TIER] [--out PATH] [--kernel]
#   run.sh --ip IP [--branch ...] [--tier ...] [--out ...] [--kernel]
#
# Defaults: --branch master  --tier all  --out ./reports/<vmid>-<ts>.md
# --kernel: enable T2.5 (runs the runner under sudo for BPF privileges)
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
# Build env-var preamble forwarded to the remote runner.
# BTF2GO_BIN and VALIDATION_ENV are always set; Datadog vars are
# included only when present on the host so empty vars aren't forwarded.
# Use printf '%q' so values with spaces or special chars don't break
# shell parsing, and pass via explicit env(1) to avoid sudoers filtering
# inline variable assignments.
remote_env_kv=("BTF2GO_BIN=/tmp/btf2go" "VALIDATION_ENV=proxmox")
[ -n "${DATADOG_API_KEY:-}" ] && remote_env_kv+=("DATADOG_API_KEY=$DATADOG_API_KEY")
[ -n "${DATADOG_SITE:-}" ]    && remote_env_kv+=("DATADOG_SITE=$DATADOG_SITE")
remote_env_q=""
for kv in "${remote_env_kv[@]}"; do
    remote_env_q+=" $(printf '%q' "$kv")"
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
# sudo -E only preserves a default whitelist, which excludes BTF2GO_BIN.
# Pass vars explicitly via env(1) under sudo so the runner can find the
# btf2go binary and emit Datadog metrics when a key is configured.
# remote_env is expanded host-side (heredoc is unquoted) before SSH.
if [ "$kernel" = 1 ]; then
    sudo env$remote_env_q /tmp/validation-runner run$runner_args_q
else
    env$remote_env_q /tmp/validation-runner run$runner_args_q
fi
EOSSH

px_log "fetching newest report + sidecar"
# Archived artifacts always land in validation/reports/ so the path
# round-trips with the in-repo archive (committable, greppable).
# --out is honored as an additional host-side copy for legacy callers.
archive_dir="$(dirname "$0")/../reports"
mkdir -p "$archive_dir"
remote_id=$(ssh -q -o StrictHostKeyChecking=accept-new \
    -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR \
    -i "${PX_SSH_KEY:-$HOME/.ssh/id_ed25519}" \
    "${PX_SSH_USER:-dani}@$ip" \
    'ls -t ~/btf2go/validation/reports/*.md 2>/dev/null \
      | grep -v "/latest_report\.md$" \
      | while read -r md; do
          [ -f "${md%.md}.json" ] || continue
          basename "$md" .md
          break
        done')
[ -n "$remote_id" ] || px_fail "no report found in validation/reports/ on $ip"
scp -q -o StrictHostKeyChecking=accept-new \
    -o UserKnownHostsFile=/dev/null \
    -o LogLevel=ERROR \
    -i "${PX_SSH_KEY:-$HOME/.ssh/id_ed25519}" \
    "${PX_SSH_USER:-dani}@$ip:btf2go/validation/reports/${remote_id}.md" \
    "${PX_SSH_USER:-dani}@$ip:btf2go/validation/reports/${remote_id}.json" \
    "$archive_dir/"
# Honor --out as a legacy single-file cache (used by older callers).
out_dir="$(dirname "$out")"
mkdir -p "$out_dir"
cp "$archive_dir/${remote_id}.md" "$out"
px_ok "archived: validation/reports/${remote_id}.{md,json} (also copied to $out)"

# Print headline so the caller can see the result inline.
sed -n '1,12p' "$archive_dir/${remote_id}.md"
