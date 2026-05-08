# Proxmox VM template for the btf2go validation runner

Builds a sealed Proxmox VM template (`btf2go-validation-tmpl`, VMID 9100)
with the same toolchain as the Daytona snapshot, so cloned VMs boot
ready to run T1–T2.5 of the validation suite without any additional
setup.

## What gets baked in

| | |
|---|---|
| OS | Debian 13 trixie |
| Kernel | 6.12 (Debian stable) |
| Go | 1.25.5 |
| Clang / LLD / LLVM | 19 (with `clang`/`llc`/`llvm-strip` symlinks in `/usr/local/bin`) |
| libbpf, bpftool | apt latest |
| Rust nightly + bpf-linker | system-wide via `/usr/local/rustup` + `/usr/local/cargo` |
| Zig | 0.16.0 |
| yq | v4.45.1 (mikefarah, not the apt Python wrapper) |
| bpf2go | pinned to cilium/ebpf v0.21.0 |
| cloud-init + qemu-guest-agent | enabled |

## Prerequisites

- HashiCorp Packer ≥ 1.10
- A Proxmox API token (Datacenter → Permissions → API Tokens)
- `../.env` populated:

  ```sh
  PROXMOX_API_URL=https://<proxmox-host>:8006/
  PROXMOX_API_TOKEN=user@realm!tokenid=<uuid-secret>
  ```

- `local:iso/debian-13.2.0-amd64-netinst.iso` already on the Proxmox node
  (verified via `pvesm list local`)

## Build

```sh
cd packer
make init       # downloads the proxmox plugin
make validate   # syntax-check proxmox.pkr.hcl
make template   # ~25 min: install + Rust nightly compile + seal
```

Packer prints a manifest at the end with the new template's VMID.

## What the build does

1. Provisions a transient VM (VMID 9100) on node `srv`, disk on
   `sata_raid_1`, network `vmbr0`, **8 vCPU / 8 GB RAM** with
   `cpu_type=host` so KVM passes through every host feature
   (matters for AVX-heavy Rust compile and BPF JIT codegen).
2. Boots the Debian netinst ISO, drives the installer with
   `preseed/preseed.cfg` served over packer's HTTP server.
3. Installs the toolchain via `scripts/01-toolchain.sh`.
4. Locks the build-time `validator` user and queues a one-shot
   service to remove it after the first cloud-init run on a clone
   (`scripts/02-validator-cleanup.sh`).
5. Generalizes machine-id / SSH host keys / cloud-init seed
   (`scripts/99-generalize.sh`).
6. Halts the VM, converts it to template **VMID 9100**.

## Cloning the template

In the Proxmox UI: right-click `btf2go-validation-tmpl` → Clone.
Configure the clone's **Cloud-Init** tab with:

- **User** — your username
- **SSH public key** — your public key (the build-time `validator`
  user is removed on first boot, so this is your only way in)
- **IP / DNS** — DHCP is fine

Boot the clone, ssh in, and:

```sh
git clone https://github.com/danigoland/btf2go.git
cd btf2go
go build -o /usr/local/bin/btf2go ./cmd/btf2go
go run ./validation/runner run --tier all --kernel
```

## Re-building after a toolchain bump

Bump the version pin in `scripts/01-toolchain.sh`, run `make template`
again. Packer will refuse to overwrite the existing 9100 — delete or
rename it first via Proxmox.

## Known caveats

- The build downloads ~3 GB during apt+rustup+cargo install. With
  a slow link the 30-min SSH timeout in `proxmox.pkr.hcl` may need
  bumping.
- `ssh_password` is a build-temporary credential, never present in
  the sealed template (the validator account is locked then deleted).
  Don't reuse it elsewhere.
- The build uses BIOS (SeaBIOS) boot, not UEFI. If you need UEFI,
  add `bios = "ovmf"` and an `efi_config { ... }` stanza to
  `proxmox.pkr.hcl` and update the boot_command for grub-EFI.
