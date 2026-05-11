#!/usr/bin/env bash
# Rebuild btf2go rust-aya fixtures locally. Not run by CI.
#
# Preconditions:
#   - rustup with `nightly-2026-04-15` installed
#   - bpf-linker 0.10.3 on PATH (`cargo install --version 0.10.3 bpf-linker`)
#
# Usage:
#   ./refresh.sh                       # rebuild all fixtures
#   ./refresh.sh maps-happy            # rebuild a single fixture
#   ./refresh.sh maps-happy multi-elf-shared
set -euo pipefail

HERE="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
TOOLCHAIN="nightly-2026-04-15"

require_tool() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "error: missing required tool: $1" >&2
    exit 1
  }
}

check_toolchain() {
  if ! rustup toolchain list | grep -q "^${TOOLCHAIN}\b"; then
    echo "error: rust toolchain ${TOOLCHAIN} not installed" >&2
    echo "       rustup toolchain install ${TOOLCHAIN}" >&2
    exit 1
  fi
}

# Build a single-crate fixture (maps-happy, maps-missing-export).
build_single() {
  local name="$1"
  local dir="${HERE}/${name}"
  if [ ! -d "${dir}" ]; then
    echo "error: fixture not found: ${dir}" >&2
    exit 1
  fi
  echo "==> rebuilding ${name}"
  (
    cd "${dir}"
    cargo "+${TOOLCHAIN}" build --target bpfel-unknown-none --release -Z build-std=core
    cp "target/bpfel-unknown-none/release/${name}" "${name}.elf"
  )
}

# Build the multi-elf workspace fixture (two binaries from one workspace).
build_multi_elf_shared() {
  local dir="${HERE}/multi-elf-shared"
  if [ ! -d "${dir}" ]; then
    echo "error: fixture not found: ${dir}" >&2
    exit 1
  fi
  echo "==> rebuilding multi-elf-shared (lsm + xdp)"
  (
    cd "${dir}"
    cargo "+${TOOLCHAIN}" build --target bpfel-unknown-none --release -Z build-std=core
    cp "target/bpfel-unknown-none/release/lsm" "lsm.elf"
    cp "target/bpfel-unknown-none/release/xdp" "xdp.elf"
  )
}

dispatch() {
  case "$1" in
    maps-happy|maps-missing-export)
      build_single "$1" ;;
    multi-elf-shared)
      build_multi_elf_shared ;;
    *)
      echo "error: unknown fixture: $1" >&2
      echo "       known: maps-happy maps-missing-export multi-elf-shared" >&2
      exit 1 ;;
  esac
}

require_tool rustup
require_tool cargo
require_tool bpf-linker
check_toolchain

if [ $# -eq 0 ]; then
  build_single maps-happy
  build_single maps-missing-export
  build_multi_elf_shared
else
  for name in "$@"; do
    dispatch "${name}"
  done
fi

echo "done."
