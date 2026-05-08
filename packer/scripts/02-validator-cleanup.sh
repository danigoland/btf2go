#!/usr/bin/env bash
# 02-validator-cleanup.sh — strip the build-time `validator` user out
# of the sealed template. Cloud-init will create whatever user is
# configured per-clone via Proxmox's cloud-init drive.
#
# The validator account exists ONLY so packer can SSH into the VM
# during provisioning. Leaving it with its known password in the
# sealed template would be a credential leak.
#
# We can't delete the user before generalize because packer is still
# logged in as them (and runs the next provisioner script via sudo).
# So we lock the account here, schedule full removal at first boot
# via a one-shot systemd unit, and clear ~/.ssh.

set -euxo pipefail

# Lock the password — login disabled but home dir survives until
# first-boot cleanup below.
passwd -l validator

# Wipe any authorized keys / shell history that might have been
# created during the build.
rm -rf /home/validator/.ssh
rm -f  /home/validator/.bash_history
truncate -s 0 /home/validator/.bash_logout || true

# Drop a one-shot service that deletes the user on the next boot,
# AFTER cloud-init has had a chance to create the per-clone user.
cat > /etc/systemd/system/btf2go-cleanup-validator.service <<'EOF'
[Unit]
Description=Remove the build-time validator user (one-shot, post cloud-init)
After=cloud-final.service
Wants=cloud-final.service
ConditionPathExists=/etc/sudoers.d/90-validator

[Service]
Type=oneshot
ExecStart=/usr/sbin/userdel -rf validator
ExecStart=/bin/rm -f /etc/sudoers.d/90-validator
ExecStart=/bin/systemctl disable btf2go-cleanup-validator.service
RemainAfterExit=no

[Install]
WantedBy=multi-user.target
EOF

systemctl enable btf2go-cleanup-validator.service

echo "[02-validator-cleanup] validator locked; first-boot service queued"
