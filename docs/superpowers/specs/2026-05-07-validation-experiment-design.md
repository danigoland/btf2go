# btf2go Validation Experiment — Design Spec

**Date:** 2026-05-07
**Status:** approved for implementation planning
**Module:** `github.com/danigoland/btf2go`
**Target:** v0.3.0 master (commit `94c0205`)

## 1. Problem Statement

btf2go has shipped through v0.3.0 with three committed-fixture goldens (C, Rust/Aya, Zig), unit tests for every codegen rule, and a layout verifier. Every fixture is hand-crafted by the maintainer. None of it proves the tool survives BTF graphs in the wild.

This document specifies a tiered experimentation program that produces an **automated validation suite** plus a **committed report artifact** giving credible evidence (not just intuition) that btf2go does what it claims.

## 2. Hypothesis Set

The program tests five falsifiable claims, each with explicit success criteria.

| # | Hypothesis | Method | Pass criterion |
|---|------------|--------|----------------|
| **H1** | For every type both tools generate, btf2go's struct layout is byte-equivalent to `bpf2go`'s | Differential — compile a C source, run both tools on the resulting ELF, AST-parse the Go outputs, intersect by sanitized name, diff `(size, fieldOffsets)` for the intersection | 100% layout match for the intersection. Disjoint sets (types one tool generates and the other doesn't) are reported but not failures — the tools have different default whitelist policies |
| **H2** | btf2go reproduces kernel-truth byte offsets for ≥99% of named types in real-world BTF-bearing ELFs | Empirical — generate Go from each corpus ELF, compile it, assert `unsafe.Sizeof`/`reflect.StructField.Offset` matches BTF | ≥99% of named structs match exactly |
| **H2.5** | A kernel-loaded eBPF program populated via btf2go-generated `SetX` accessors is read back by Go via `GetX` accessors with byte-equal field values | End-to-end — Proxmox VM, real kernel, BPF map round-trip with a deliberately tricky `WireT` struct | Every field in `WireT` round-trips |
| **H3** | btf2go produces compilable, layout-correct Go for every published Aya/Rust eBPF project we test | Per-project build + generate + compile + layout-verify | Compiles for ≥80% of the corpus (Aya toolchain churn means 100% is unrealistic) |
| **H4** | A new user can integrate btf2go end-to-end into a fresh Aya project in ≤30 min using only README + docs/aya-quickstart.md | Self-conducted UX walkthrough with timestamps + friction log | Successful round-trip ≤30 min wall-clock |

## 3. Tier Catalog

### T1 — Differential vs `bpf2go` (H1)

Strongest signal because we're comparing against a tool users already trust.

- **Corpus:** ~5–8 small `.c` files. Sources:
  - `cilium/ebpf` testdata (e.g., `testdata/loader.c`, `testdata/btf-map-init.c`)
  - 2–3 `libbpf-tools` programs we can compile from source
- **Workflow per project:**
  1. Compile `.c → .o` with `clang -g -target bpf -O2`
  2. Run `bpf2go pkgname source.c -- $cflags` and capture `<stem>_bpfel.go`
  3. Run `btf2go generate --elf source.o --pkg pkgname --out btf2go.go --type <each-type>`
  4. AST-parse both Go files, build maps `name → (size, [field → offset])`
  5. Diff. Names may differ (sanitization style); layout must match per-field.
- **Output:** Per-project diff table; aggregate "M of N structs match exactly."

### T2 — Empirical layout correctness on real ELFs (H2)

Wider corpus, cheaper per project (no parallel `bpf2go` invocation), looser pass bar.

- **Corpus:** ~20 ELFs from public eBPF projects (mix of:
  - Linux kernel selftests (extracted from a kernel build)
  - `libbpf-tools` released binaries
  - `cilium/ebpf` test ELFs already in the dependency
  - the 3 we already commit
- **Workflow per ELF:**
  1. Parse BTF → `(typeName → expectedSize, [field → expectedOffset])`
  2. Run `btf2go generate` for every named struct
  3. Compile generated Go as a temp package
  4. Use `unsafe.Sizeof` and `reflect.StructField.Offset` to read actual layout
  5. Diff
- **Output:** "Out of {N} structs across {M} ELFs, {X} match, {Y} don't, breakdown by category (anonymous, bitfield, union, etc.)."

### T2.5 — Kernel verifier round-trip (H2.5)

Strongest possible correctness claim. Requires a real Linux kernel, so runs only on the Proxmox VM execution target.

- **Corpus:** A single bespoke kernel program written for this experiment (`validation/runner/kernel/wire.bpf.c`). The `WireT` struct deliberately exercises:
  - Mix of `u8`/`u16`/`u32`/`u64`/signed equivalents
  - A 3-bitfield run packed into a single byte
  - A nested union with `u64 raw` and `struct { u32 lo, u32 hi } pair`
  - A `[16]char comm` array
  - A packed `uint64` at offset 4 (forces our packed-downgrade path)
- **Workflow:**
  1. On dev machine: compile `wire.bpf.c` with clang, run `btf2go generate`, commit golden output to `validation/runner/wirepkg/wire.go`
  2. On Proxmox VM (root): SSH, copy the ELF and the test program, run `go test`. Test:
     - Loads ELF via `ebpf.LoadCollectionSpec` + `NewCollection` (no attach)
     - Populates a `BPF_MAP_TYPE_HASH` map keyed by a single u32 with a populated `WireT` value
     - Triggers a userspace BPF iteration helper to read the map back
     - Asserts byte-equality between the populated value and the read-back value
     - Asserts every accessor's `Get` returns what `Set` wrote
- **Output:** PASS / FAIL plus the populated and read-back hex bytes for visual comparison.
- **Caveats:** Kernel version sensitivity — pin the VM to a known kernel and document it.

### T3 — Aya/Rust ecosystem coverage (H3)

Pitch validation. Real Aya projects with non-trivial type graphs.

- **Corpus (target 5–8 projects, all Aya kernel-side):**
  - `aya-rs/aya-template` default scaffold (control)
  - `aya-rs/book` example programs (xdp, kprobe, lsm, tc — pick one per type)
  - `kunai-project/kunai` (production threat-detection tool, real type graph)
  - `bpfman/bpfman` if its kernel side uses Aya
  - `redbpf-rs` examples if compatible
  - The hand-written stress fixture (`tests/fixtures/rust/`) as a baseline anchor
- **Workflow per project:**
  1. Clone at pinned commit, run the project's own `cargo +nightly build` (with our `DYLD_FALLBACK_LIBRARY_PATH` workaround on macOS, or natively on Daytona)
  2. Run `btf2go inspect --verbose` to enumerate user-defined types
  3. Run `btf2go generate --type <each-named-struct>` and capture stdout-equivalent
  4. Compile the generated Go as a standalone Go package
  5. Where possible, layout-verify against BTF (T2-style)
- **Output:** Per-project status table: built? generated? compiled? layout-verified? + crash/error categorization.

### T4 — Self-conducted UX walkthrough (H4)

Squishy by design. UX issues only surface when the author stops being the author.

- **Corpus:** One greenfield Aya project, scaffolded fresh with `cargo generate` from `aya-template`.
- **Workflow:** Time-boxed walkthrough with a stopwatch:
  1. Define a struct with a deliberate mix of features (bitfield-equivalent, union, enum, array, nested struct)
  2. Build kernel ELF
  3. Run `btf2go inspect` (verbose) on it
  4. Run `btf2go generate`
  5. Write a 30-line Go consumer that loads + reads from a map
  6. Build, run (or smoke against committed ELF), confirm round-trip
- **What to log:** Every "wait, why is it doing X" moment, every doc lookup, every error encountered.
- **Output:** Annotated transcript at `validation/runner/ux/transcript.md` with timestamps and ticket list.

## 4. Architecture

### Directory layout

```
validation/
├── corpus/
│   ├── manifest.yaml           # one entry per corpus project: name, source-url, pinned commit, build cmd
│   ├── c/                      # T1 + T2: cilium/ebpf testdata, libbpf-tools, etc.
│   └── aya/                    # T3: Aya/Rust projects
├── runner/
│   ├── main.go                 # cobra-style entrypoint: --tier {1,2,2.5,3,all}
│   ├── tier1_diff.go
│   ├── tier2_layout.go
│   ├── tier2_5_kernel.go       # invoked only with --kernel flag (Proxmox VM)
│   ├── tier3_aya.go
│   ├── kernel/wire.bpf.c       # T2.5 fixture source
│   ├── kernel/wire.elf         # committed compiled artifact
│   ├── wirepkg/wire.go         # btf2go-generated golden for T2.5
│   ├── ux/transcript.md        # T4 hand-written log, runner reads + summarizes
│   └── report.go               # aggregates results into validation/report.md
├── .devcontainer/
│   ├── devcontainer.json       # Daytona / VS Code Dev Containers spec
│   └── Dockerfile              # debian:trixie + clang-21 + libbpf-dev + rustup
│                               #     nightly + bpf-linker + zig 0.16
├── SETUP.md                    # 3 paths: macOS-local / Daytona / Proxmox VM
├── refresh.sh                  # clones/updates corpus/ from manifest.yaml
└── report.md                   # generated artifact, committed
```

### Key decisions

- **Corpus is not committed to git.** `manifest.yaml` lists source URLs + pinned commits. `refresh.sh` materializes locally. The manifest IS committed.
- **Runner is one Go program**, not Makefile-driven shell. Each tier is a function returning structured `[]Finding`.
- **Tiers are independent.** Tier 2 still runs even if Tier 1 has 50 mismatches. Each contributes a report section.
- **T4 stays human.** Markdown log committed by hand; runner reads it and includes a summary + transcript link in the unified report.
- **Each tier probes for required tools** (`exec.LookPath`) and returns `Skipped{Reason}` if missing rather than failing the run. Skips are reported as "skipped — toolchain missing" so a reader sees what's incomplete.
- **`.devcontainer/`** is the canonical "everything-installed" environment. Opening the repo in Daytona auto-builds and gives a shell with `clang`, `cargo +nightly`, `bpf-linker`, `zig`, `bpftool`, `llvm-objdump` all on `PATH`.

### Execution environments

| Env | Best for | Why |
|-----|----------|-----|
| macOS local | T1, T4. Quick runner iteration. | Free, partial toolchain (no full BPF kernel). |
| Daytona | T1+T2+T3. Reproducible CI-equivalent runs. The report's headline numbers come from here. | Clean Linux container, ~2-min spin-up, no kernel privileges. |
| Proxmox VM | T1+T2+T3+T2.5. Persistent corpus cache. | Persistent, root, real kernel with BPF support. |

The runner is environment-agnostic. `--kernel` (or detection of `/sys/fs/bpf` + privileges) gates T2.5.

## 5. Report Structure

`validation/report.md` is the artifact — what gets pull-quoted into a launch post or referenced from the README.

```markdown
# btf2go validation report

Generated: 2026-05-07
btf2go: v0.3.0 (commit 94c0205)

## Headline
{X / Y named structs verified across {Z projects} produce
byte-correct Go layouts. {Y - X} mismatches; details below.}

## Tier 1 — Differential vs bpf2go
| Project | Structs | Match | Mismatch |
| ------- | ------- | ----- | -------- |
| ...

## Tier 2 — Empirical layout correctness
... per-ELF, types, size match, offset match, mismatch detail

## Tier 2.5 — Kernel round-trip (Proxmox VM)
PASS / FAIL + populated/read-back hex dumps

## Tier 3 — Aya/Rust ecosystem coverage
... per-project: built? generated? compiled? layout verified?

## Tier 4 — UX walkthrough
... summary + link to transcript.md

## Skipped tiers
| Tier | Reason |
| ---- | ------ |
```

## 6. Bugs Found ≠ Failure

If T1/T2/T2.5 surface bugs in btf2go, **the bugs are the deliverable**. Fix them, re-run, the report's headline numbers improve. The validation suite becomes the regression gate for v0.3.x patches and v0.4.0.

Most likely surfaces:
- T1 (vs `bpf2go`) is most likely to find naming-convention disagreements (we sanitize differently); these are reported but don't fail the run.
- T2 / T2.5 are most likely to find Phase 4 alignment bugs that the unit tests' mock-IR coverage missed. The currently-committed goldens have only been spot-checked; T2 will exhaustively cross-check every named struct against BTF.
- T3 is most likely to find unsupported BTF kinds (e.g., `Func`, `FuncProto`, complex `Datasec` patterns) — these are user-visible v0.4 work.

## 7. Out of Scope (v0.3 validation pass)

- Performance benchmarking (btf2go vs bpf2go runtime)
- Fuzzing (random valid BTF graphs in/out)
- Recruited external user studies (T4 is self-conducted)
- Integration into upstream `cilium/ebpf` or `aya-rs` test suites
- Pre-tagged binary distribution

These are v0.4+ if they become priorities.

## 8. Time Budget

Multi-day pass — single hacker, ~2–4 days wall-clock.

| Tier | Effort |
|------|--------|
| Runner skeleton + manifest plumbing | ~half day |
| T1 implementation + corpus curation | ~half day |
| T2 implementation + corpus curation | ~half day |
| T2.5 kernel program + Proxmox plumbing | ~full day |
| T3 corpus build + per-project build scripts | ~half day |
| T4 self-walkthrough + transcript | ~2 hrs |
| Report aggregation + writing | ~2 hrs |

If T2.5 (or any tier) blows up in scope, defer it without blocking the others.

## 9. Open Questions

- **Proxmox VM kernel version pin?** Need to pick one (probably linux-6.10+ for modern BTF features) and lock it in `validation/SETUP.md`.
- **`bpf2go` invocation in T1**: `bpf2go` expects a `.c` source path, not a pre-compiled `.elf`. The runner needs to know the C source for each T1 corpus entry, plus the `clang` flags. Manifest should carry both.
- **What if T2.5's bespoke kernel program needs root on macOS local?** It does. Runner should hard-skip T2.5 unless `--kernel` is passed AND the runner has CAP_BPF. macOS local can never run T2.5.
