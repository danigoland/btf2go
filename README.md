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

Map key/value types are auto-included. Use `--type` (repeatable) to add more types beyond the map seeds. Pass `--no-map-types` to opt out of map auto-inclusion. `--type` accepts the raw BTF name, the terminal segment of a `::`-path, or the Go-sanitized form — `btf2go inspect --elf X --names` prints all three.

The generated file is single, self-contained, gofmt-clean, and includes its own `Pointer[T any] uint64` declaration so consumers don't take a runtime dependency on `btf2go` itself. The `// Source:` header defaults to the ELF basename — pass `--source-name <str>` to embed an explicit deterministic identifier instead.

### Rust + aya users

Pass `--aya` to unwrap aya map wrappers (`HashMap<K, V>`, `LruHashMap<K, V>`, `Array<V>`) and emit Go for the value types. Extend the bridge for custom wrappers via `--aya-bridge Name=arity:positions` (repeatable).

For aya's map V types to appear in BTF, force them in with the [`btf2go-aya-export`](./aya-export) helper crate:

```rust
use btf2go_aya_export::btf_export;
btf_export!(Foo, Bar);
```

See [`docs/aya-maps.md`](./docs/aya-maps.md) for the full guide and [`docs/aya-quickstart.md`](./docs/aya-quickstart.md) for toolchain setup.

### Multi-ELF projects

When two ELFs share types in the same Go package, route shared types to one file with `--shared-out <path>` + `--shared-type <name>` (repeatable). `Pointer[T]` always lives in the shared file when active. `--shared-type` computes the transitive closure of dependencies automatically, and bitfield accessor methods travel with their struct.

```sh
btf2go generate --elf lsm.elf --pkg bpfgen --aya \
  --shared-out internal/bpfgen/shared.go \
  --shared-type BinaryIdentity \
  --out internal/bpfgen/lsm_types.go

btf2go generate --elf xdp.elf --pkg bpfgen --aya \
  --shared-out internal/bpfgen/shared.go \
  --shared-type BinaryIdentity \
  --out internal/bpfgen/xdp_types.go
```

## What you get

- Structs with all alignment-correct padding and packed-struct downgrades
- Enums (signed and unsigned) as `type Foo intN` + `const` blocks
- Bitfields collapsed into `[N]byte` storage with generated `Get<Field>` / `Set<Field>` accessor methods
- Unions as `[MaxSize]byte` storage with `As<Member>` / `SetAs<Member>` accessor methods (uses `unsafe.Pointer`)
- Pointers as a phantom-typed `Pointer[Target] uint64` wrapper
- Sanitized identifiers — Rust namespace-mangled names like `my_module::MyEvent` become `MyModuleMyEvent`
- Aya map wrappers (`HashMap<K, V>`, `LruHashMap<K, V>`, `Array<V>`) auto-unwrapped to their value types via `--aya`
- Deterministic output — same ELF in, byte-identical Go out, regardless of build host

## Status

**v0.5.0.** Supports structs, arrays, pointers, enums (signed and unsigned), bitfields with accessor methods, unions with typed-view accessors, `btf.Var` resolution, function-pointer field degradation, aya map-wrapper unwrapping, and multi-ELF shared-type dedup. Validated against the cilium/ebpf testdata corpus and real Rust/aya + C ELFs (all automated tiers green).

Out of scope: loader scaffolding (`*ebpf.CollectionSpec` generation, embedded ELF, typed map handles, `Load*()` functions). Keep using `cilium/ebpf` directly for loading — it works fine for any-language ELFs once you have the right struct types.

Endianness: BTF carries no endianness marker for value bytes. Run `btf2go` on a host with the same endianness as your deployment target. In practice this means little-endian on every realistic eBPF host (linux/amd64, linux/arm64); big-endian targets like s390x are not currently tested.

## License

Apache-2.0.
