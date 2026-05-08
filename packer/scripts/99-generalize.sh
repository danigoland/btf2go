#!/usr/bin/env bash
# 99-generalize.sh — final seal before packer converts the VM into a
# Proxmox template. Removes per-instance state so each clone boots
# with a fresh identity (machine-id, SSH host keys, cloud-init seed).

set -euxo pipefail
export DEBIAN_FRONTEND=noninteractive

# Reset cloud-init so the next boot re-runs all stages against the
# Proxmox cloud-init drive (which is per-clone).
cloud-init clean --logs --seed --machine-id || true

# SSH host keys regenerate at first boot via openssh-server's
# preset rules.
rm -f /etc/ssh/ssh_host_*

# machine-id regenerates from systemd-machine-id-setup at boot.
truncate -s 0 /etc/machine-id
truncate -s 0 /var/lib/dbus/machine-id 2>/dev/null || true

# Clear logs and apt cache (smaller template).
journalctl --rotate || true
journalctl --vacuum-time=1s || true
apt-get clean
rm -rf /var/lib/apt/lists/* /var/cache/apt/archives/*.deb

# Clear shells / temp.
rm -rf /tmp/* /var/tmp/* /root/.cache /root/.bash_history
find /home -maxdepth 2 -name '.bash_history' -delete || true

# Zero free space so the qcow2/zvol can be sparsified by ZFS later.
# Don't fail the build if there's no room.
dd if=/dev/zero of=/zero bs=1M status=none 2>/dev/null || true
sync
rm -f /zero

echo "[99-generalize] sealed; ready for shutdown"
