# Validation suite execution environment. Install everything
# every tier needs so the runner can run with no skips.
FROM debian:trixie

LABEL org.opencontainers.image.title="btf2go-validation"

ENV DEBIAN_FRONTEND=noninteractive
ENV CARGO_HOME=/usr/local/cargo
ENV RUSTUP_HOME=/usr/local/rustup
ENV PATH=/usr/local/cargo/bin:/usr/local/go/bin:$PATH

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        clang-19 lld-19 llvm-19 llvm-19-dev libclang-19-dev \
        libbpf-dev linux-libc-dev \
        bpftool \
        curl git make pkg-config \
        jq \
        zlib1g-dev libelf-dev \
    && rm -rf /var/lib/apt/lists/* \
    && ln -sf /usr/bin/clang-19 /usr/local/bin/clang \
    && ln -sf /usr/bin/llc-19 /usr/local/bin/llc \
    && ln -sf /usr/bin/llvm-strip-19 /usr/local/bin/llvm-strip

# Go 1.25.5
RUN curl -fsSL https://go.dev/dl/go1.25.5.linux-amd64.tar.gz \
        | tar -C /usr/local -xzf -

# Rust nightly + bpf-linker
RUN curl -fsSL https://sh.rustup.rs | sh -s -- -y --profile minimal --default-toolchain nightly \
    && rustup component add rust-src --toolchain nightly \
    && cargo install bpf-linker --version 0.10.3 --locked

# Zig 0.16.0 (matches spec; pinned stable release)
RUN curl -fsSL https://ziglang.org/download/0.16.0/zig-x86_64-linux-0.16.0.tar.xz \
        | tar -C /usr/local -xJf - \
    && ln -s /usr/local/zig-x86_64-linux-0.16.0/zig /usr/local/bin/zig

# yq v4 from mikefarah (apt's "yq" package is the Python v3 wrapper
# with jq-style syntax; refresh.sh needs v4's "@tsv" / pipeline syntax)
RUN curl -fsSL -o /usr/local/bin/yq \
        https://github.com/mikefarah/yq/releases/download/v4.45.1/yq_linux_amd64 \
    && chmod +x /usr/local/bin/yq

# bpf2go pinned to the cilium/ebpf version we validate against
# in the c_corpus, so reproducible runs use a single tool version.
RUN go install github.com/cilium/ebpf/cmd/bpf2go@v0.21.0

WORKDIR /workspace
