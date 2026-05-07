# btf2go

Generate Go structs from compiled eBPF ELF artifacts via embedded BTF.

`btf2go` reads BTF debug info directly from `.elf` / `.o` files produced by any eBPF toolchain (clang, rustc/Aya, zig) and emits Go type definitions whose memory layout matches the kernel's view of those types exactly.

It is **not** a `bpf2go` replacement. It complements `cilium/ebpf`'s `bpf2go` by removing the C-source-and-clang step from the generation pipeline: feed in any pre-built ELF, get out Go types. Use `bpf2go` for the "I write C and want a full Go skeleton" path. Use `btf2go` for the "I built my eBPF program in Rust/Zig/CO-RE and just need correct Go types" path.

## Install

```sh
go install github.com/danigoland/btf2go/cmd/btf2go@latest
```

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

**v0.1.0 (alpha).** Supports structs, arrays, pointers, enums, signed and unsigned ints, bitfields with accessor methods, and unions with typed-view accessors.

Out of scope for v0.1: loader scaffolding (`*ebpf.CollectionSpec` generation, embedded ELF, typed map handles, `Load*()` functions). Keep using `cilium/ebpf` directly for loading — it works fine for any-language ELFs once you have the right struct types.

Endianness: BTF carries no endianness marker for value bytes. Generate against the same endianness as your deployment target. In practice this means little-endian on every realistic eBPF host (linux/amd64, linux/arm64).

## License

Apache-2.0.
