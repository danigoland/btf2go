# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Status

This repository currently contains **only design documents** — no Go code has been scaffolded yet:

- `btf2go Architecture Document.md` — the canonical 5-phase pipeline spec
- `btf2go Implementation Plan.md` — milestones, directory layout, testing strategy

When starting implementation, follow the directory layout in the Implementation Plan (cmd/btf2go, internal/{btfparser,types,align,generator}, pkg/btf2go, tests/{rust_bpf,zig_bpf}).

## What btf2go Is

A CLI that generates Go structs from compiled eBPF ELF artifacts by reading embedded **BTF** (BPF Type Format) data — not source code. The point is to be language-agnostic (Rust/Aya, Zig, C all produce BTF) so Go user-space orchestrators can interop with non-C kernel programs without manual struct alignment.

Differentiator vs. `cilium/ebpf`'s `bpf2go`: `bpf2go` parses C headers; `btf2go` parses the BTF graph from the compiled `.elf`/`.o`. This is the entire reason the project exists — do not regress to source-parsing approaches.

## Architecture: The 5-Phase Pipeline (Critical)

The architecture is a strict pipeline. Each phase has a single responsibility; do not collapse phases.

1. **Ingestion** (`internal/btfparser`) — `btf.LoadSpecFromELF()` from `github.com/cilium/ebpf/btf` → `btf.Spec`.
2. **Target Resolution & Sanitization** (`internal/btfparser`) — Walk `btf.Map` keys/values plus `--type` whitelist; recursively resolve deps; **discard kernel noise** (task_struct, sk_buff, etc.). Sanitize mangled names like `my_module::MyStruct` → `MyModuleMyStruct`.
3. **Type Traversal & Union Engine** (`internal/types`) — Build a Go IR (`GoStruct`, `GoField`, `GoEnum`). `btf.Int` mapped by **size** to uintN. Pointers become `btf2go.Pointer[T]` (typed wrapper, not bare uint64). Unions become `[MAX_SIZE]byte` plus accessor methods (v0.2.0).
4. **Memory Alignment Calculator** (`internal/align`) — **The critical path.** See rules below.
5. **Code Generation** (`internal/generator`) — `text/template` → `go/format.Source()`. On format failure, print unformatted output to stderr for debugging.

## Alignment Rules (Phase 4) — Get These Right

These rules exist because the LLVM/C memory model and Go's `gc` memory model disagree. Bugs here corrupt user data silently.

- **Bits → bytes:** BTF offsets are in bits. `byteOffset = field.Offset / 8`.
- **Bitfields:** If `field.Offset % 8 != 0`, the struct contains bitfields. **Downgrade the whole bitfield block to a raw `[X]byte` array** — do not try to emit Go bitfield-like fields.
- **Implicit padding:** Track `go_cursor`. If `btf_offset > go_cursor`, LLVM inserted padding. Inject `_ [N]byte` padding field into the IR.
- **Packed-struct conflict:** If a primitive (e.g., `uint64`) lands at an offset that violates Go's natural alignment (Go would silently insert padding to fix it), **downgrade the primitive to `[8]byte`** to force the exact layout. Always check `expected_btf_offset == go_current_offset` after each field.

## Testing Strategy

- **Unit tests** for `internal/align` using mock `btf.Struct` objects with packed/bitfield/padding edge cases.
- **E2E cross-compiler tests** in `tests/`: compile the same complex struct in C (clang), Rust (rustc/Aya), and Zig → run btf2go → compile generated Go → assert with `unsafe.Offsetof` / `unsafe.Sizeof` on every field. The test suite is expected to invoke `clang`, `rustc`, and `zig` itself.

## Dependencies (planned)

- `github.com/cilium/ebpf/btf` — BTF parser (do not reinvent ELF/BTF reading)
- `github.com/spf13/cobra` — CLI
- stdlib `text/template` and `go/format`
