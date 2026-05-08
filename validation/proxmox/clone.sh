#!/usr/bin/env bash
# clone.sh — clone the btf2go-validation template into a fresh VM,
# attach cloud-init config (user + SSH key + DHCP), boot, wait for
# IP. Prints "VMID IP" to stdout.
#
# Usage: clone.sh [NAME]
#   NAME defaults to btf2go-val-<short-uuid>
#
# Env overrides:
#   PROXMOX_TEMPLATE_VMID   default 9100
#   PX_SSH_USER             cloud-init username (default: dani)
#   PX_SSH_KEY_PUB          path to public key (default: ~/.ssh/id_ed25519.pub)
#   PROXMOX_VMID_RANGE_LO   default 9110
#   PROXMOX_VMID_RANGE_HI   default 9199

set -euo pipefail
source "$(dirname "$0")/lib.sh"
px_init

name="${1:-btf2go-val-$(uuidgen 2>/dev/null | cut -c1-8 || echo $$)}"
ssh_user="${PX_SSH_USER:-dani}"
ssh_key_pub="${PX_SSH_KEY_PUB:-$HOME/.ssh/id_ed25519.pub}"

vmid=$(px_next_vmid)
px_log "cloning template $PROXMOX_TEMPLATE_VMID -> $vmid ($name)"

clone_upid=$(px_api POST "/nodes/$PROXMOX_NODE/qemu/$PROXMOX_TEMPLATE_VMID/clone" \
    -d "newid=$vmid&name=$name&full=0" | jq -r '.data')
px_task "$clone_upid"

px_log "configuring cloud-init (user=$ssh_user, dhcp)"
sshkeys_enc=$(px_sshkey_b64 "$ssh_key_pub")
config_upid=$(px_api POST "/nodes/$PROXMOX_NODE/qemu/$vmid/config" \
    --data-urlencode "ipconfig0=ip=dhcp" \
    --data-urlencode "ciuser=$ssh_user" \
    --data "sshkeys=$sshkeys_enc" | jq -r '.data')
px_task "$config_upid"

px_log "starting VM $vmid"
start_upid=$(px_api POST "/nodes/$PROXMOX_NODE/qemu/$vmid/status/start" | jq -r '.data')
px_task "$start_upid"

px_log "waiting for QEMU agent to report IP"
ip=$(px_ip "$vmid")
px_ok "VM $vmid up at $ip"
echo "$vmid $ip"
