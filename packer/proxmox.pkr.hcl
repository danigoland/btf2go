// Packer build for the btf2go validation Proxmox VM template.
// Driven by `make template` in this directory; reads PROXMOX_API_URL
// and PROXMOX_API_TOKEN from ../.env (sourced before invocation).
//
// Output: a sealed Proxmox template (VM 9100, name btf2go-validation-tmpl)
// with Go 1.25.5, clang 19, libbpf, rust nightly + bpf-linker, zig 0.16.0,
// yq v4, and bpf2go @ v0.21.0 — the same toolchain as the Daytona snapshot.

packer {
  required_plugins {
    proxmox = {
      version = ">= 1.2.3"
      source  = "github.com/hashicorp/proxmox"
    }
  }
}

// PROXMOX_API_TOKEN format: "user@realm!tokenid=secret".
// We split once on "=" to get (token_id, token_secret).
variable "proxmox_api_url" {
  type    = string
  default = env("PROXMOX_API_URL")
}

variable "proxmox_api_token" {
  type      = string
  default   = env("PROXMOX_API_TOKEN")
  sensitive = true
}

locals {
  token_parts          = split("=", var.proxmox_api_token)
  proxmox_token_id     = local.token_parts[0]
  proxmox_token_secret = local.token_parts[1]

  // The validator user is only used during packer's SSH-based
  // provisioning. The build password is rotated and the account
  // is removed in 99-generalize.sh so the sealed template forces
  // the operator to provision an SSH user via cloud-init.
  build_user     = "validator"
  build_password = "btf2go-build-temp"
}

source "proxmox-iso" "btf2go_validation" {
  proxmox_url              = "${trimsuffix(var.proxmox_api_url, "/")}/api2/json"
  insecure_skip_tls_verify = true
  username                 = local.proxmox_token_id
  token                    = local.proxmox_token_secret

  node                 = "srv"
  vm_id                = 9100
  vm_name              = "btf2go-validation-build"
  template_name        = "btf2go-validation-tmpl"
  template_description = "btf2go validation runner. Go 1.25.5, clang 19, libbpf, rust nightly + bpf-linker, zig 0.16.0, yq v4.45.1, bpf2go @ v0.21.0. Built by packer/proxmox.pkr.hcl. Cloud-init enabled — set user/SSH key via Proxmox before clone boots."

  // Existing Debian 13 netinst already on the host.
  boot_iso {
    type             = "scsi"
    iso_file         = "local:iso/debian-13.2.0-amd64-netinst.iso"
    iso_storage_pool = "local"
    unmount          = true
  }

  // Build-time resources — Rust nightly + bpf-linker compile is the peak.
  cpu_type = "host"
  cores    = 8
  sockets  = 1
  memory   = 8192

  // Disk: ZFS-backed, raw format to skip qcow2 layer.
  scsi_controller = "virtio-scsi-pci"
  disks {
    disk_size    = "25G"
    storage_pool = "sata_raid_1"
    type         = "scsi"
    format       = "raw"
    discard      = true
    ssd          = true
  }

  network_adapters {
    bridge = "vmbr0"
    model  = "virtio"
  }

  // Cloud-init drive, attached to the sealed template.
  cloud_init              = true
  cloud_init_storage_pool = "sata_raid_1"

  // Useful for shutdown via shutdown_command and IP discovery.
  qemu_agent = true

  // Boot the netinst installer with a preseed served from packer's
  // built-in HTTP server. The boot prompt is the legacy isolinux
  // menu; "auto" tells d-i to fetch preseed.cfg over HTTP.
  boot_wait    = "5s"
  http_directory = "preseed"
  boot_command = [
    "<esc><wait>",
    "auto url=http://{{ .HTTPIP }}:{{ .HTTPPort }}/preseed.cfg",
    " net.ifnames=0 biosdevname=0",
    " hostname=btf2go-validation domain=local",
    "<enter>"
  ]

  // SSH credentials match what the preseed creates for `validator`.
  ssh_username = local.build_user
  ssh_password = local.build_password
  ssh_timeout  = "30m"
}

build {
  name    = "btf2go-validation"
  sources = ["source.proxmox-iso.btf2go_validation"]

  // Toolchain install — sudo because we mutate /usr/local + apt.
  provisioner "shell" {
    execute_command = "echo '${local.build_password}' | sudo -S env {{ .Vars }} bash '{{ .Path }}'"
    scripts = [
      "scripts/01-toolchain.sh",
      "scripts/02-validator-cleanup.sh",
      "scripts/99-generalize.sh"
    ]
  }
}
