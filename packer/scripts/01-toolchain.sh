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
ln -sf /usr/sbin/bpftool      /usr/local/bin/bpftool

# --- Go 1.25.5 ---------------------------------------------------------
GO_VERSION=1.25.5
curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" \
  | tar -C /usr/local -xzf -

ln -sf /usr/local/go/bin/go    /usr/local/bin/go
ln -sf /usr/local/go/bin/gofmt /usr/local/bin/gofmt

# Toolchain envs for ALL shells (login + non-login + sudo). /etc/environment
# is read by pam_env, so SSH `ssh host 'cmd'` invocations get them too —
# /etc/profile.d/* would only apply to login shells.
cat >> /etc/environment <<EOF
RUSTUP_HOME=/usr/local/rustup
CARGO_HOME=/usr/local/cargo
GOTOOLCHAIN=local
EOF

# Also export for this script — /etc/environment is read by PAM at
# login, not by the currently-running shell, so subsequent curl |
# rustup-init below would otherwise default to $HOME/.cargo.
export RUSTUP_HOME=/usr/local/rustup
export CARGO_HOME=/usr/local/cargo
export GOTOOLCHAIN=local

# --- Rust nightly + bpf-linker (system-wide) --------------------------
curl -fsSL https://sh.rustup.rs | \
  sh -s -- -y --profile minimal --default-toolchain nightly --no-modify-path

/usr/local/cargo/bin/rustup component add rust-src --toolchain nightly
# Pin bpf-linker so reproducible runs see the same code-generator.
/usr/local/cargo/bin/cargo install bpf-linker --version 0.10.3 --locked

# Make the rust install world-readable so non-root users (post-clone
# cloud-init users) can use it without sudo.
chmod -R a+rX /usr/local/rustup /usr/local/cargo

# Symlink rust entry points into /usr/local/bin so SSH non-login
# shells (default PATH) find them without sourcing profile.
for b in rustc cargo rustup bpf-linker rustdoc; do
  if [ -e "/usr/local/cargo/bin/$b" ]; then
    ln -sf "/usr/local/cargo/bin/$b" "/usr/local/bin/$b"
  fi
done

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
