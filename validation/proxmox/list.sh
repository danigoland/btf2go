#!/usr/bin/env bash
# list.sh — print every VM in the validation clone range with status,
# IP (if reported), and clock time.

set -euo pipefail
source "$(dirname "$0")/lib.sh"
px_init

px_api GET "/cluster/resources?type=vm" | jq -r --argjson lo "$PROXMOX_VMID_RANGE_LO" --argjson hi "$PROXMOX_VMID_RANGE_HI" --argjson t "$PROXMOX_TEMPLATE_VMID" '
.data
| map(select((.vmid >= $lo and .vmid <= $hi) or .vmid == $t))
| sort_by(.vmid)
| (["VMID","NAME","STATUS","UPTIME"] | @tsv),
  (.[] | [.vmid, .name, (if .vmid == $t then "TEMPLATE" else .status end),
          (.uptime // 0 | tostring)] | @tsv)
' | column -t -s $'\t'
