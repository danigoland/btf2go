# Changelog

All notable changes to this project will be documented in this file. The format follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/) and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

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

[Unreleased]: https://github.com/danigoland/btf2go/compare/v0.1.0...HEAD
[v0.1.0]: https://github.com/danigoland/btf2go/releases/tag/v0.1.0
