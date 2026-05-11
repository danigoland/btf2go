# btf2go

[![ci](https://github.com/danigoland/btf2go/actions/workflows/ci.yml/badge.svg)](https://github.com/danigoland/btf2go/actions/workflows/ci.yml)
[![release](https://img.shields.io/github/v/release/danigoland/btf2go?display_name=tag&sort=semver)](https://github.com/danigoland/btf2go/releases)
[![go reference](https://pkg.go.dev/badge/github.com/danigoland/btf2go.svg)](https://pkg.go.dev/github.com/danigoland/btf2go)
[![apache 2.0](https://img.shields.io/badge/license-Apache--2.0-blue.svg)](LICENSE)

Generate Go structs from compiled eBPF ELF artifacts via embedded BTF.

`btf2go` reads BTF debug info directly from `.elf` / `.o` files **with embedded BTF** produced by any eBPF toolchain (clang, rustc/Aya, zig) and emits Go type definitions whose memory layout matches the kernel's view of those types exactly. ELF artifacts compiled without `clang -g` (or the equivalent) are rejected with a clear error.

It is **not** a `bpf2go` replacement. It complements `cilium/ebpf`'s `bpf2go` by removing the C-source-and-clang step from the generation pipeline: feed in any pre-built ELF, get out Go types. Use `bpf2go` for the "I write C and want a full Go skeleton" path. Use `btf2go` for the "I built my eBPF program in Rust/Zig/CO-RE and just need correct Go types" path.

## Install

**Requirements:** Go 1.24 or newer (`cilium/ebpf v0.21` requires Go 1.24; the range-over-func iterator API it exposes requires Go 1.23+).

```sh
go install github.com/danigoland/btf2go/cmd/btf2go@latest
```

> **Note (Linux):** `go install` places compiled binaries in `$HOME/go/bin`. If that directory isn't on your `$PATH`, add `export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin` to your shell profile and re-source it.

## Usage

```sh
btf2go generate \
  --elf path/to/program.elf \
  --pkg events \
  --out events_gen.go \
  --type my_event_t
```

Map key/value types are auto-included. Use `--type` (repeatable) to add more types beyond the map seeds. Pass `--no-map-types` to opt out of map auto-inclusion.

The generated file is single, self-contained, gofmt-clean, and includes its own `Pointer[T any] uint64` declaration so consumers don't take a runtime dependency on `btf2go` itself.

## What you get

- Structs with all alignment-correct padding and packed-struct downgrades
- Enums (signed and unsigned) as `type Foo intN` + `const` blocks
- Bitfields collapsed into `[N]byte` storage with generated `Get<Field>` / `Set<Field>` accessor methods
- Unions as `[MaxSize]byte` storage with `As<Member>` / `SetAs<Member>` accessor methods (uses `unsafe.Pointer`)
- Pointers as a phantom-typed `Pointer[Target] uint64` wrapper
- Sanitized identifiers — Rust namespace-mangled names like `my_module::MyEvent` become `MyModuleMyEvent`

## Status

**v0.3.1.** Supports structs, arrays, pointers, enums, signed and unsigned ints, bitfields with accessor methods, unions with typed-view accessors, `btf.Var` resolution, and function-pointer field degradation. Validated against the cilium/ebpf testdata corpus and a real-kernel BPF map round-trip (19 PASS / 0 FAIL across all automated tiers).

Out of scope: loader scaffolding (`*ebpf.CollectionSpec` generation, embedded ELF, typed map handles, `Load*()` functions). Keep using `cilium/ebpf` directly for loading — it works fine for any-language ELFs once you have the right struct types.

Endianness: BTF carries no endianness marker for value bytes. Run `btf2go` on a host with the same endianness as your deployment target. In practice this means little-endian on every realistic eBPF host (linux/amd64, linux/arm64); big-endian targets like s390x are not currently tested.

## License

Apache-2.0.
