# Build the btf2go validation Proxmox VM template.
#
# Usage:
#   make init      # one-time: download the proxmox plugin
#   make template  # build + seal the VM 9101 template
#   make validate  # syntax-check proxmox.pkr.hcl
#   make clean     # drop packer cache
#
# Reads PROXMOX_API_URL and PROXMOX_API_TOKEN from ../.env.

PACKER ?= packer
PACKER_LOG ?= 0
ENV_FILE := $(realpath ../.env)

# Sourcing the env file before each invocation keeps secrets out of
# Make's environment and out of any subprocess that doesn't need them.
RUN := set -a && . $(ENV_FILE) && set +a && PACKER_LOG=$(PACKER_LOG)

.PHONY: init validate template clean

init:
	@test -f $(ENV_FILE) || { echo "missing ../.env (PROXMOX_API_URL, PROXMOX_API_TOKEN)"; exit 1; }
	$(RUN) && $(PACKER) init proxmox.pkr.hcl

validate:
	$(RUN) && $(PACKER) validate proxmox.pkr.hcl

template:
	@test -f $(ENV_FILE) || { echo "missing ../.env (PROXMOX_API_URL, PROXMOX_API_TOKEN)"; exit 1; }
	$(RUN) && $(PACKER) build proxmox.pkr.hcl

clean:
	rm -rf packer_cache/ manifest.json *.box
