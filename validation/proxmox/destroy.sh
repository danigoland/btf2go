#!/usr/bin/env bash
# destroy.sh — stop and purge a validation clone.
#
# Usage: destroy.sh VMID [VMID...]
#        destroy.sh --all          # purge every VM in the clone range

set -euo pipefail
source "$(dirname "$0")/lib.sh"
px_init

if [ "${1:-}" = "--all" ]; then
    mapfile -t vmids < <(px_api GET "/cluster/resources?type=vm" \
        | jq -r --argjson lo "$PROXMOX_VMID_RANGE_LO" --argjson hi "$PROXMOX_VMID_RANGE_HI" \
              '.data[] | select(.vmid >= $lo and .vmid <= $hi) | .vmid')
    [ ${#vmids[@]} -gt 0 ] || { px_log "no clones to destroy"; exit 0; }
else
    [ $# -gt 0 ] || { sed -n '3,7p' "$0" >&2; exit 1; }
    vmids=("$@")
fi

for vmid in "${vmids[@]}"; do
    [ "$vmid" = "$PROXMOX_TEMPLATE_VMID" ] && {
        px_log "refusing to destroy template VMID $vmid"; continue
    }
    px_log "stopping $vmid"
    stop_upid=$(px_api POST "/nodes/$PROXMOX_NODE/qemu/$vmid/status/stop" 2>/dev/null \
        | jq -r '.data // ""')
    [ -n "$stop_upid" ] && px_task "$stop_upid" || true
    px_log "destroying $vmid"
    rm_upid=$(px_api DELETE "/nodes/$PROXMOX_NODE/qemu/${vmid}?purge=1" | jq -r '.data')
    px_task "$rm_upid"
    px_ok "$vmid purged"
done
