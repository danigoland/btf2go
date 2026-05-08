#!/usr/bin/env bash
# validate.sh — full lifecycle: clone -> run -> fetch report -> destroy.
#
# Usage:
#   validate.sh [--branch BRANCH] [--tier TIER] [--keep] [--out PATH]
#
# Defaults:
#   --branch master   --tier all   destroy after run unless --keep
#
# Examples:
#   validate.sh --tier 2
#   validate.sh --branch feature/foo --keep
#   validate.sh --tier 2 --tier 4   (repeat --tier; passed through to runner)

set -euo pipefail
source "$(dirname "$0")/lib.sh"
px_init

branch="master" tiers=() keep=0 out=""
while [ $# -gt 0 ]; do
    case "$1" in
        --branch) branch="$2"; shift 2 ;;
        --tier)   tiers+=("$2"); shift 2 ;;
        --keep)   keep=1; shift ;;
        --out)    out="$2"; shift 2 ;;
        --help|-h) sed -n '3,12p' "$0"; exit 0 ;;
        *)         px_fail "unknown arg: $1" ;;
    esac
done
[ ${#tiers[@]} -gt 0 ] || tiers=(all)

# Clone+config+boot.
read -r vmid ip <<<"$(bash "$(dirname "$0")/clone.sh")"

cleanup() {
    local rc=$?
    if [ "$keep" = 0 ] && [ -n "${vmid:-}" ]; then
        px_log "cleanup: destroying $vmid"
        bash "$(dirname "$0")/destroy.sh" "$vmid" || true
    elif [ "$keep" = 1 ]; then
        px_ok "kept VM $vmid (ip $ip) — destroy with: bash $(dirname "$0")/destroy.sh $vmid"
    fi
    exit "$rc"
}
trap cleanup EXIT

# Run validation; first --tier wins, repeated --tier accumulate.
run_args=(--branch "$branch")
for t in "${tiers[@]}"; do run_args+=(--tier "$t"); done
[ -n "$out" ] && run_args+=(--out "$out")

bash "$(dirname "$0")/run.sh" --ip "$ip" "${run_args[@]}"
