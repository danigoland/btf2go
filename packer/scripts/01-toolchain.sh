#!/usr/bin/env bash
# 01-toolchain.sh — install the validation runner's toolchain into the
# build VM. Mirrors validation/.devcontainer/Dockerfile so the Proxmox
# template and the Daytona snapshot stay in lockstep.

set -euxo pipefail
export DEBIAN_FRONTEND=noninteractive

# --- native packages ---------------------------------------------------
apt-get update -qq
apt-get install -y --no-install-recommends \
    build-essential ca-certificates \
    clang-19 lld-19 llvm-19 llvm-19-dev libclang-19-dev \
    libbpf-dev linux-libc-dev bpftool \
    curl git make pkg-config jq xz-utils \
    zlib1g-dev libelf-dev

ln -sf /usr/bin/clang-19      /usr/local/bin/clang
ln -sf /usr/bin/llc-19        /usr/local/bin/llc
ln -sf /usr/bin/llvm-strip-19 /usr/local/bin/llvm-strip

# --- Go 1.25.5 ---------------------------------------------------------
GO_VERSION=1.25.5
curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
  | tar -C /usr/local -xzf -

# System-wide PATH and toolchain envs for every login shell.
cat > /etc/profile.d/btf2go-toolchain.sh <<'EOF'
export PATH=/usr/local/go/bin:/usr/local/cargo/bin:$PATH
export CARGO_HOME=/usr/local/cargo
export RUSTUP_HOME=/usr/local/rustup
export GOTOOLCHAIN=local
EOF
chmod +x /etc/profile.d/btf2go-toolchain.sh
# shellcheck source=/dev/null
. /etc/profile.d/btf2go-toolchain.sh

# --- Rust nightly + bpf-linker (system-wide) --------------------------
curl -fsSL https://sh.rustup.rs | \
  sh -s -- -y --profile minimal --default-toolchain nightly --no-modify-path

/usr/local/cargo/bin/rustup component add rust-src --toolchain nightly
/usr/local/cargo/bin/cargo install bpf-linker

# Make the rust install world-readable so non-root users (post-clone
# cloud-init users) can use it without sudo.
chmod -R a+rX /usr/local/rustup /usr/local/cargo

# --- Zig 0.16.0 -------------------------------------------------------
curl -fsSL https://ziglang.org/download/0.16.0/zig-x86_64-linux-0.16.0.tar.xz \
  | tar -C /usr/local -xJf -
ln -sf /usr/local/zig-x86_64-linux-0.16.0/zig /usr/local/bin/zig

# --- yq v4.45.1 (mikefarah; apt's yq is the Python v3 wrapper) -------
curl -fsSL -o /usr/local/bin/yq \
  https://github.com/mikefarah/yq/releases/download/v4.45.1/yq_linux_amd64
chmod +x /usr/local/bin/yq

# --- bpf2go pinned to cilium/ebpf v0.21.0 ----------------------------
GOPATH=/tmp/gopath PATH=/usr/local/go/bin:$PATH \
  go install github.com/cilium/ebpf/cmd/bpf2go@v0.21.0
mv /tmp/gopath/bin/bpf2go /usr/local/bin/bpf2go
rm -rf /tmp/gopath

# Smoke check — versions for the build log.
clang --version | head -1
go version
/usr/local/cargo/bin/rustc --version
/usr/local/cargo/bin/bpf-linker --version
zig version
yq --version
bpf2go --help 2>&1 | head -1 || true

echo "[01-toolchain] done"
