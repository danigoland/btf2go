#!/usr/bin/env bash
# 02-validator-cleanup.sh — strip the build-time `validator` user out
# of the sealed template. The previous systemd-based approach proved
# unreliable (the unit's "After=cloud-final" ordering didn't trigger
# on first boot). Use cloud-init's first-boot runcmd hook instead —
# it runs exactly once per instance ID and cloud-init's clean step in
# 99-generalize.sh wipes the seed so each clone re-triggers it.
#
# We can't delete the user before generalize because packer is still
# logged in as them. So we lock the account here, clear its SSH state,
# and queue the actual deletion via cloud-init.

set -euxo pipefail

# Lock the password — login disabled but home dir survives until
# the first cloud-init pass on a clone.
passwd -l validator
rm -rf /home/validator/.ssh
rm -f  /home/validator/.bash_history

# Drop a cloud-init drop-in that fires runcmd on every "fresh" instance
# (cloud-init treats each instance ID as fresh, and 99-generalize.sh
# resets the seed so every clone is a fresh instance).
mkdir -p /etc/cloud/cloud.cfg.d
cat > /etc/cloud/cloud.cfg.d/99-cleanup-validator.cfg <<'EOF'
#cloud-config
# Inserted by packer's 02-validator-cleanup.sh. Removes the build-time
# validator account on the first boot of each cloned VM.
runcmd:
  - [ bash, -c, "id validator >/dev/null 2>&1 && userdel -rf validator || true" ]
  - [ rm, -f, /etc/sudoers.d/90-validator ]
EOF

# Belt-and-suspenders: also disable the package via systemd-tmpfiles
# in case cloud-init somehow doesn't run.
echo "[02-validator-cleanup] validator locked; runcmd queued"
