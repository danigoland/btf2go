# Changelog

All notable changes to this project will be documented in this file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [v0.5.0] — 2026-05-10

Closes four follow-up gaps surfaced by FireLXC's CI integration after v0.4.0.
Three affect the `--shared-out` / `--shared-type` pipeline; one eliminates
manual boilerplate for Aya map value type exports.

### Added

- **`btf2go-aya-export` helper crate** (`aya-export/`) ships the
  `btf_export!(T, …)` macro. One call forces each map value type into the
  ELF's BTF section as a standalone entry so `btf2go generate --aya` can
  resolve it. Replaces the hand-written `#[no_mangle] static _BTF_EXPORT_*`
  statics that were required in v0.4.0. The macro expands to a
  `#[no_mangle] static _BTF_EXPORT_<NAME>: MaybeUninit<T>` per type;
  `MaybeUninit::uninit()` is `const fn` so no `Default` impl is needed.
  The crate itself compiles on stable Rust. (Gap 1)

### Fixed

- **`--shared-out` files are now byte-identical across identical-input runs.**
  v0.4.0 embedded a `(run at <RFC3339>)` timestamp on each source-path line,
  causing every regeneration to produce a different file. The timestamp annotation
  is dropped; source-path provenance is retained. `parseSharedFile` is
  backward-compatible with v0.4.x files that still carry the suffix. (Gap 8)

- **`--shared-type Foo` now computes the transitive closure of referenced types.**
  If `Foo` has fields of type `Bar` (or `[N]Bar`, etc.), `Bar` is automatically
  routed to the shared file without the user listing it explicitly. The walk is
  depth-first and cycle-aware (`Pointer[T]` wrappers are skipped as terminals).
  Enums and unions reachable through the closure are also hoisted; unions that
  use `unsafe.Pointer` in their accessors trigger `import "unsafe"` in the shared
  file automatically. (Gap 9)

- **`--shared-type` now preserves bitfield `Get`/`Set` accessor methods.**
  v0.4.0's shared-file rescanner stopped at the struct's closing `}` and dropped
  any following `func (s *Foo) ...` method blocks. The methods now travel with
  the struct into the shared file. `renderStructOnly` formats its output (so
  re-parsed bodies and freshly rendered bodies are whitespace-identical and
  shape-mismatch checks stay accurate). (Gap 10)

### Internal

- `internal/generator/shared.go` — `parseSharedFile` tracks an `inSourcesBlock`
  flag to correctly distinguish source-path comment lines from other comments;
  scans past the struct's closing `}` to capture following methods.
- `internal/generator/generate.go` — `extendSharedSet`, `referencedGeneratedTypes`,
  `renderEnumOnly`, `renderUnionOnly` helpers added; filtered file now excludes
  shared enums and unions in addition to shared structs.

## [v0.4.0] — 2026-05-10

Closes seven gaps surfaced by FireLXC's integration of btf2go into a
Rust/aya Cargo workspace with a Go user-space loader. After this
release, FireLXC's `xtask::normalize_btf2go_header` helper, its
`dedup_go_types` helper, its `--no-map-types`/`--type` arrays in
`gen_go_for`, and the per-eBPF-crate `#[no_mangle] _BTF_EXPORT_*`
statics can all be deleted.

### Added

- **`generate --aya`** decodes Rust generic-instantiation BTF names
  (`HashMap_3C_…_3E_` → `HashMap<…>`) and unwraps aya map wrappers,
  seeding the layout-bearing value type into the resolution closure.
  Default bridge supports `HashMap`, `LruHashMap`, and `Array`. (Gap 1)
- **`generate --aya-bridge Name=arity:positions`** (repeating) extends
  or overrides the bridge table for custom wrappers / V-only producers.
  Implies `--aya`. (Gap 1)
- **`generate --shared-out <path>`** emits `Pointer[T]` and any
  `--shared-type` entries to a shared file, enabling multiple ELFs to
  share a Go package without duplicate-declaration errors. (Gap 2)
- **`generate --shared-type <name>`** (repeating) routes a type to the
  shared file. Requires `--shared-out`. v0.4 limitation: only struct
  types are supported in the shared-types pipeline; enum and union
  hoisting is a future extension. (Gap 2)
- **`generate --source-name <str>`** overrides the `// Source:` header
  value with a deterministic identifier. Without it, the header now
  defaults to the ELF basename (was the verbatim `--elf` path). Keeps
  generated files diff-stable across build hosts when the
  source-of-truth pattern is "commit the generated file, gate PRs on
  no-drift". (Gap 6)
- **`inspect --names`** prints raw / Go-sanitized / terminal-segment
  names for each named type, so users can read off the right
  `--type` argument value. (Gap 3)
- **`docs/aya-maps.md`** — user guide for `--aya`, `--aya-bridge`,
  `--shared-out`, `--shared-type`, `--source-name`, and the
  `_BTF_EXPORT_*` workaround. (Gap 1)
- **`docs/aya-quickstart.md`** — new "How to produce a BTF-bearing ELF"
  section with rustc-aya / clang / zig examples and the `build-std`
  footgun call-out. (Gap 5)
- **`tests/fixtures/rust-aya/`** — three new fixtures (`maps-happy`,
  `maps-missing-export`, `multi-elf-shared`) with goldens and a
  `refresh.sh` rebuild script. End-to-end coverage in
  `tests/rust_aya_test.go`.

### Changed

- **`generate --type`** now accepts any of: raw BTF name, terminal
  segment of `::`-paths, sanitized exact, or sanitized terminal.
  Collision detection on same-tier ambiguity. (Gap 3)
- **`// Source:` header** now defaults to `filepath.Base(--elf)` instead
  of the verbatim path. Pass `--source-name` to override. **Breaking
  change to generated file contents**: re-running `btf2go generate` on
  any existing project will produce a one-line diff on `// Source:`.
  Regenerate and commit the new headers, or pass `--source-name` to
  pin to the prior value. (Gap 6)
- **BTF-less ELF diagnostic** now keys guidance by toolchain (Rust/aya
  rustflags + `link-arg=--btf`, clang `-g`, zig pointer to
  `docs/aya-quickstart.md#zig-toolchain`). Replaces the
  bpf-linker/LLVM-mismatch paragraph from v0.3.2's PR #66. (Gap 4)
- **`go.mod` floor** lowered from `go 1.25.5` to `go 1.24.0`
  (cilium/ebpf v0.21.0's floor). Broadens the install base for
  downstream consumers on older Go toolchains. README's install section
  now says "Requires Go 1.24 or newer". (Gap 7)

### Internal

- `internal/btfparser/decode.go` — pure `_3C_…_3E_` → `<…>` decoder.
- `internal/btfparser/lookup.go` — four-tier `LookupTypeByName` with
  `LookupNotFoundError` / `LookupAmbiguousError`.
- `internal/btfparser/bridge.go` — aya bridge + override parser.
- `internal/generator/shared.go` — shared-file scanner + merge.
- `internal/types.GoFile.OmitPointer` field to suppress per-ELF
  `Pointer[T]` emission when SharedOut is active.
- `internal/align` continues to have zero `btf` imports.

## [v0.3.2] — 2026-05-10

Patch release. Bug fix + ergonomics — surfaced by the T4 multi-model UX walkthrough sweep.

### Fixed

- **`btf2go inspect` no longer silently emits empty output for BTF-less ELFs.** When bpf-linker's LLVM version doesn't match the system LLVM (a common Aya/Rust pitfall — bpf-linker v0.10.3 needs LLVM-22 but many distros ship LLVM-19), the build "succeeds" but produces an ELF without a `.BTF` section. Previously `btf2go inspect` printed nothing and the user had no signal. Now returns non-zero with an actionable error pointing at bpf-linker version + a `readelf -S` verification command. (#66)

### Added

- **`btf2go version` subcommand** + matching `--version` flag — previously errored with "unknown flag". (#65)
- **`btf2go generate` prints `Generated: <path>` to stderr** on success — previously silent, leaving fresh users unsure whether the command did anything. (#65)
- **Aya quickstart guide updates** — five doc gaps surfaced by the T4 walkthrough sweep, including aya-build BTF emission requirements, the C-only nature of bitfield accessors, and the `asm/types.h` sysroot gotcha on Debian. (#64)
- **T4 validation framework: multi-model methodology** — the validation runner's T4 tier (UX walkthrough) now has a documented multi-model evaluation path (M2.7 + Kimi K2.6 + Qwen3-Coder-Next) producing a comparison report at `validation/runner/ux/transcripts/comparison.md`. The friction-aware prompt and Proxmox-VM Ollama-Cloud bridging are documented for reproducibility.

## [v0.3.1] — 2026-05-08

Patch release. Two correctness fixes uncovered by the new validation experiment running btf2go against the cilium/ebpf testdata corpus.

### Fixed

- **`ARRAY_OF_MAPS` / `HASH_OF_MAPS` inner-map types** — the `.maps` Datasec resolver only chased `key` and `value` members, missing inner-map type references encoded as `__array(values, struct foo)` in libbpf-style outer-map definitions. The inner type now lands in the resolved set; the existing transitive-closure walker pulls in its own `key`/`value` for free. (#40)
- **Function-pointer fields no longer crash btf2go** — struct fields whose BTF chain reached `*btf.FuncProto` or `*btf.Func` previously aborted the entire run with `unsupported BTF type`. Common in BPF `struct_ops` ELFs (function pointers in ops vtables). They now degrade to `Pointer[uintptr]` (binary-correct 8 bytes on 64-bit), preserving the containing struct's other field offsets. (#39)

### Added

- **Validation framework + first archived report** — `validation/runner/` is a tiered experiment (T1 differential vs `bpf2go`, T2 empirical layout, T2.5 kernel round-trip on Linux, T3 Aya/Rust ecosystem, T4 UX walkthrough) with per-run report archive at `validation/reports/<id>.{md,json}`. The post-fix baseline on master is **19 PASS / 0 FAIL / 18 SKIP** across 5 tiers including a real-kernel BPF map round-trip on a Proxmox VM (T2.5). (#37, #38, #41, #42, #43)

## [v0.3.0] — 2026-05-07

Minor release. Closes the highest-priority gaps from the post-v0.2.0 backlog review and validates the language-agnostic claim end-to-end.

### Added

- **`btf.Var` support** — `--type SOME_RODATA_CONST` (or any other Var name) now resolves through the Var wrapper to its underlying type instead of erroring "unsupported BTF type". CO-RE / Aya programs that put compile-time constants in `.rodata` can now generate Go-side struct definitions for them.
- **`btf2go inspect --verbose`** (or `-v`) — expands DATASEC entries to show their vars and the underlying type each points at, with a tree-style indented row per var.
- **Zig fixture + golden** — `tests/fixtures/zig/` parallel to C and Rust. Validates btf2go works on rustc-emitted, clang-emitted, AND zig-emitted BTF graphs. The README's "any eBPF toolchain" claim now has three independent proofs.

### Fixed

- **Union backing-storage alignment (correctness)** — Generated `type Foo struct { _data [N]byte }` had `Alignof = 1`, so a standalone union value at a non-aligned address would SIGBUS on ARM64/RISC-V/MIPS when the `As<Member>` accessor cast to `*uint64`. Now uses `[N/8]uint64` (or smaller depending on the union's max-member alignment) so the cast is always alignment-safe. Same size, correct alignment metadata.
- `traverse`: rename `max` shadow of the Go 1.21+ predeclared builtin.

[v0.4.0]: https://github.com/danigoland/btf2go/releases/tag/v0.4.0
[v0.3.2]: https://github.com/danigoland/btf2go/releases/tag/v0.3.2
[v0.3.1]: https://github.com/danigoland/btf2go/releases/tag/v0.3.1
[v0.3.0]: https://github.com/danigoland/btf2go/releases/tag/v0.3.0
[Unreleased]: https://github.com/danigoland/btf2go/compare/v0.4.0...HEAD

## [v0.2.0] — 2026-05-07

Minor release. New CLI surface and three usability fixes driven by stress-testing v0.1.2 against a richer Aya/Rust BTF graph.

### Added

- **`btf2go inspect <elf>`** — new subcommand that lists every named struct, union, enum, and datasec in a BTF graph without generating Go code. `--filter <substring>` for case-insensitive substring matching. Useful for diagnosing "why isn't `--type Foo` resolving."
- **"Did you mean" suggestions** when `--type X` doesn't resolve. Resolver now wraps the cilium/ebpf "not found" error in a typed `*TypeNotFoundError` carrying up to 3 close matches, ordered by Levenshtein distance. Implements `Unwrap()` so `errors.Is/As` still works against the underlying error.
- **`btf.Int` with `Encoding == btf.Bool` OR `Name == "bool"`** now renders as Go `bool` instead of `uint8`. Same layout, idiomatic source. Catches both clang `_Bool` (encoding-flagged) and Rust `bool` (name-only). Discovered via stress-testing.

### Changed

- Reorganized `cmd/btf2go/main.go` to host the `inspect` subcommand alongside `generate`. Kept the existing `generate` flag set unchanged.

[v0.2.0]: https://github.com/danigoland/btf2go/releases/tag/v0.2.0

## [v0.1.2] — 2026-05-07

Patch release. Acts on the post-v0.1.1 self-review of master.

### Fixed

- `renderBitAccessor` now refuses 64-bit bitfields whose bit offset is not byte-aligned (would span 9 bytes; the `uint64` accumulator can't hold it). Emits the same unsupported stub used for >64-bit widths instead of silently truncating the high bits.
- CLI: `--pkg` is now validated against `^[a-z_][a-z0-9_]*$` **and** rejected when it matches a Go reserved keyword (`type`, `var`, `func`, etc.). Catches obvious mistakes upfront with a clear error instead of waiting for `go build` on the generated file to fail.
- `AnonName` no longer calls `strings.TrimPrefix(p, "")` (was a no-op leftover).

### Changed

- Generated output no longer emits the defensive `var _ = unsafe.Sizeof(uintptr(0))` line. The union accessor methods always reference `unsafe`, so the import is naturally live whenever it's emitted. Goldens regenerated.
- `classifyKind` switched from raw slice comparisons to `strings.HasPrefix` (cosmetic).

### Removed

- `internal/generator/templates/file.tmpl` — orphaned since v0.1.0 PR #7 round 1 when codegen moved fully to `strings.Builder`.

### Tests

- `TestGenerateSanitizesHeader`: confirms newline injection through `opts.Source` AND `opts.ToolVersion` cannot break out of the leading `//` header block.
- `TestRenderBitAccessorRefuses64BitMisaligned`: locks in the new guard for the 64-bit non-byte-aligned bitfield case.

## [v0.1.1] — 2026-05-07

### Added

- `btf.Float` → `float32` (size 4) / `float64` (size 8) / `[N]byte` fallback for unusual sizes (e.g. `long double`).
- `btf.TypeTag` passthrough in `traverse.declare()` (matches the existing closure walker).
- `classifyKind` recognizes `float32`/`float64`/`uintptr` as primitive kinds, so the alignment downgrade path applies to misaligned floats.
- Aya/Rust kernel fixture under `tests/fixtures/rust/` (Cargo.toml, `.cargo/config.toml`, `src/main.rs`, committed `fixture.elf`, golden under `eventspkg/events.go`).
- Table-driven `TestGolden` with `c` and `rust` subtests.
- `examples/aya-roundtrip/` — Aya counterpart of `c-roundtrip`. Validates rustc-nightly + bpf-linker → btf2go → cilium/ebpf end-to-end.
- README badges (CI status, latest release, pkg.go.dev, license).
- This `CHANGELOG.md`.
- Fixture Makefile: probe both Apple-silicon (`/opt/homebrew`) and Intel (`/usr/local`) Homebrew prefixes for `clang` and `libLLVM`.

### Fixed

- `Generate(nil, opts)` now returns a clear error instead of panicking.
- Layout verifier handles a missing `EventsT` key in the JSON sidecar with a `t.Fatal` instead of silently passing.
- Layout verifier probes host endianness and skips the union `{Lo, Hi}` decomposition assertion on big-endian.
- Codegen sanitizes `opts.Source` and `opts.ToolVersion` to prevent newline injection into the generated file's leading comment.
- CLI surfaces cobra `Get*` flag-read errors with context instead of discarding them.
- `toolVersion()` treats `(devel)` (the documented placeholder for non-tagged builds) the same as missing and falls back to `v0.1.1-dev`.

## [v0.1.0] — 2026-05-07

First release. Generates Go structs from BTF embedded in compiled eBPF ELF artifacts (clang, rustc/Aya, zig).

### Added

- **CLI** — `btf2go generate --elf <path> --pkg <name> --out <file> [--type <name>...] [--no-map-types]`. Required flags fail loudly. Single-file output.
- **Phase 1 — Ingestion** (`internal/btfparser/load.go`): wraps `btf.LoadSpec`. Auto-detects ELF vs raw `.btf`. Errors if the input has no embedded BTF.
- **Phase 2 — Resolve + Sanitize** (`internal/btfparser/resolve.go`, `sanitize.go`): whitelist closure of `--type` ∪ `.maps` Datasec key/value types ∪ recursive deps; unwraps qualifier wrappers (`Const`/`Volatile`/`Restrict`/`Typedef`/`TypeTag`) via `btf.UnderlyingType`. Sanitizer maps Rust namespace mangling (`my_module::Foo` → `MyModuleFoo`) and other non-Go-identifier characters into PascalCase.
- **Phase 3 — Traversal** (`internal/traverse/traverse.go`): converts BTF type graph to IR. Handles primitives, arrays, pointers, enums (signed and unsigned with sign-aware `Underlying`), structs, unions, void, and qualifier unwrap. Anonymous structs/unions get `<Parent>Anon<N>` names. Bitfield members tagged with absolute `BitOffset` and `BitfieldBits` for Phase 4 to consume.
- **Phase 4 — Alignment** (`internal/align/align.go`): per-struct `Apply` walks fields, inserts synthetic `_padN` fields for implicit padding, downgrades misaligned primitives to `[N]byte` to defeat Go's automatic alignment, and collapses contiguous bitfield runs into a single `_bfN [N]byte` storage field plus accessor metadata. Errors loudly on overlapping fields and on summed-bytes-exceed-declared-size.
- **Phase 5 — Codegen** (`internal/generator/generate.go`): renders the IR to gofmt-clean source via `strings.Builder` and `go/format.Source`. On format failure, the unformatted source is dumped to stderr and still written to disk. Header values are sanitized to prevent newline injection. Generates `Pointer[T any] uint64`, signed/unsigned enum constants (with sign-extension for signed enums), union accessors using `unsafe.Pointer`, and bitfield Get/Set accessor methods (handles cross-byte runs, signed sign-extension).
- **End-to-end test** (`tests/`): C fixture `events.c` exercising padding, bitfield run, array, nested struct, and union. Committed `.elf`, `.golden.go`, and `.layout.json`. The golden test diffs btf2go output against the committed golden; the layout verifier compiles the golden as a real Go package and asserts every field with `unsafe.Offsetof` plus a bitfield round-trip with cross-field non-corruption checks. Endianness is probed at runtime; big-endian hosts skip the union-decomposition assertion.
- **Example demo** (`examples/c-roundtrip/`): runnable program that loads the fixture ELF via `cilium/ebpf`, looks up `events_t` in the BTF graph, marshals an `EventsT` to bytes, and round-trips the primitives, bitfields, and union accessors.
- **CI** (`.github/workflows/ci.yml`): build/vet/`go test -race` matrix on `ubuntu-latest`, `ubuntu-24.04-arm`, `macos-latest`. `govulncheck` job. All third-party actions pinned to commit SHA. Job timeouts.

### Out of scope (future)

- Loader / `*ebpf.CollectionSpec` generation (use `cilium/ebpf` directly).
- `btf.Float`, `btf.Func` / `btf.FuncProto`, `btf.Datasec` exposure.
- Rust/Aya and Zig fixtures in CI (toolchain-coupled).
- Big-endian targets (s390x).

[v0.1.2]: https://github.com/danigoland/btf2go/releases/tag/v0.1.2
[v0.1.1]: https://github.com/danigoland/btf2go/releases/tag/v0.1.1
[v0.1.0]: https://github.com/danigoland/btf2go/releases/tag/v0.1.0
