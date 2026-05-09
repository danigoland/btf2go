# Validation suite setup

The runner is environment-agnostic — it probes for required tools
and skips experiments whose toolchains aren't present. The three
environments below differ in how complete a run they support.

## Environment 1 — macOS local

**Supports:** T1 (with bpf2go installed), T2 (with clang), T3
(rebuild Aya projects locally — slow, requires
`DYLD_FALLBACK_LIBRARY_PATH` for bpf-linker), T4. **Cannot run T2.5.**

```sh
brew install llvm yq jq
brew install zig                               # for any zig fixtures
rustup install nightly
cargo install bpf-linker
go install github.com/cilium/ebpf/cmd/bpf2go@v0.21.0
go install ./cmd/btf2go                        # from repo root

# materialize the corpus
bash validation/refresh.sh

# run the partial suite (no kernel)
cd validation/runner
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/opt/llvm/lib \
BTF2GO_BIN="$(go env GOPATH)/bin/btf2go" \
  go run . run --tier all
cat ../report.md
```

## Environment 2 — Daytona (recommended)

The canonical execution target. Reproducible Debian trixie container
with every toolchain pre-installed via `validation/.devcontainer/`.

### One-time: build the snapshot

```sh
daytona snapshot create btf2go-validation:3 \
  --dockerfile validation/.devcontainer/Dockerfile \
  --cpu 4 --memory 8 --disk 10
```

Expect ~5–10 min for the rust-nightly + bpf-linker compile to finish.
Bump the `:3` tag whenever the Dockerfile changes; old tags can be
removed with `daytona snapshot delete <tag>`.

### Each run: launch a sandbox from the snapshot

```sh
# Spin up a fresh sandbox using the snapshot
daytona sandbox create --name btf2go-run --snapshot btf2go-validation:3

# Get the sandbox ID and shell in
SBX=$(daytona sandbox list --label name=btf2go-run -o id)
daytona sandbox exec $SBX -- bash

# Inside the sandbox:
git clone https://github.com/danigoland/btf2go.git
cd btf2go
git checkout feature/validation-runner   # or master once merged
bash validation/refresh.sh
go build -o /usr/local/bin/btf2go ./cmd/btf2go
cd validation/runner
go run . run --tier all
cat ../report.md
```

T2.5 is skipped in Daytona because the container has no
`/sys/fs/bpf` (no kernel BPF in containerized runtimes). For T2.5
use Environment 3.

## Environment 3 — Proxmox VM (T2.5 + the rest)

Persistent Linux VM with kernel BPF support. Required for T2.5. Pin
the kernel to a known version so results are reproducible.

The recommended path is the **prebuilt VM template** at
`packer/proxmox.pkr.hcl` (build it once with `cd packer && make
template`) plus the **orchestrator scripts** at `validation/proxmox/`
which automate the full clone → run → destroy loop:

```sh
cd validation/proxmox
./validate.sh                  # clone -> run all tiers -> fetch report -> destroy
./validate.sh --tier 2         # T2 only
./validate.sh --keep           # leave the clone running
./list.sh                      # tabular view of clones
./destroy.sh --all             # nuke every clone in the validation range
```

See `validation/proxmox/README.md` for full details. The manual
walkthrough below is for users who want to skip the orchestrator
or build the VM from scratch.

```sh
# Inside the VM (Debian/Ubuntu example):
sudo apt install -y clang lld llvm libbpf-dev linux-libc-dev bpftool \
                    build-essential pkg-config curl git make
# yq v4 (apt's yq is the Python v3 wrapper — wrong syntax)
sudo curl -fsSL -o /usr/local/bin/yq \
  https://github.com/mikefarah/yq/releases/download/v4.45.1/yq_linux_amd64
sudo chmod +x /usr/local/bin/yq
# Plus rustup nightly + bpf-linker, zig 0.16, Go 1.25 — see
# validation/.devcontainer/Dockerfile for exact versions.

# Mount /sys/fs/bpf if not auto-mounted
sudo mount -t bpf bpf /sys/fs/bpf

# Run as root to load the test program
sudo BTF2GO_BIN="$(go env GOPATH)/bin/btf2go" \
  go run ./validation/runner run --tier all --kernel
```

**Kernel version pin:** confirmed working on linux-6.10+. Earlier
kernels may lack BPF features that `wire.bpf.c` uses (T2.5 is
implemented in Tasks 8-9; until those land, `--kernel` produces a
single SKIP placeholder).

## Optional: Datadog telemetry

After each archived run the runner can emit per-run metrics and an event
to Datadog. Set `DATADOG_API_KEY` to enable; omit it (or leave it empty)
to skip silently — the validation result is never affected by Datadog
reachability.

```sh
export DATADOG_API_KEY=<your-key>
export DATADOG_SITE=datadoghq.com   # optional; defaults to datadoghq.com
                                    # use datadoghq.eu for EU tenants, etc.
```

**Metrics emitted (v2 series API, gauge):**

| Metric | Tags |
|--------|------|
| `btf2go.validation.findings.pass` | env, host, arch, tag, dirty |
| `btf2go.validation.findings.fail` | env, host, arch, tag, dirty |
| `btf2go.validation.findings.skip` | env, host, arch, tag, dirty |
| `btf2go.validation.tier.pass_rate` | + `tier:t1` … `tier:t4` |
| `btf2go.validation.tier.findings_total` | + `tier:t1` … `tier:t4` |

`commit:<sha>` is on the event (not gauges) to keep custom-metric
cardinality bounded for long-running dashboards.

**Event emitted (v1 events API):**
- Title: `btf2go validation run on <env>: <pass>/<fail>/<skip>`
- `alert_type`: `info` (no fails) or `warning` (any fails)
- Tags include `commit:<short-sha>` for direct navigation from the
  event stream to the source commit.

No SDK dependency — standard library only (`net/http`, `encoding/json`).

## Smoke test

After running in any environment:

```sh
head -30 validation/report.md
```

Expect a "Headline" section followed by per-tier sections. Skipped
tiers are reported with a clear reason — that's expected, not a
failure. A representative output (Daytona, no Aya/T2.5):

```text
# btf2go validation report

Generated: 2026-05-08
btf2go: v0.3.0 (commit b6a0bc3)

## Headline

37 findings: **9 PASS**, **11 FAIL**, 17 SKIP across 5 tiers.
```
