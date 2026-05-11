# tests/fixtures/rust-aya/maps-happy

Aya eBPF crate exercising three `#[map]` declarations:

- `HashMap<u64, ScaffoldPing>`
- `Array<BinaryIdentity>`
- `LruHashMap<u32, FooEvent>`

Plus a non-map struct `BareStruct` referenced via an explicit `--type` request
(exercises the existing non-aya emission path in the same fixture).

The `#[no_mangle] static _BTF_EXPORT_*` constants force the V types
into BTF as standalone entries so the v0.4 `--aya` bridge can resolve
them by their plain names via terminal-segment fallback.

## Goldens

Each golden lives in its own subdirectory (Go package per directory rule):

- `golden_no_aya/types.go` — generated with `btf2go generate --pkg fixture --type BareStruct`
- `golden_aya/types.go` — generated with `btf2go generate --pkg fixture --aya`

**Key difference (Gap 1 before/after):**

Without `--aya`, btf2go sees the `maps` DATASEC entries as their raw mangled
wrapper types (`HashMap_3C_u64_2C__20_maps_happy_3A__3A_ScaffoldPing_3E_`, etc.)
which have no meaningful Go fields to emit. The V types (`ScaffoldPing`,
`BinaryIdentity`, `FooEvent`) are not auto-discovered.

With `--aya`, the bridge decodes each mangled map type name, extracts the V type
via terminal-segment lookup, and adds it to the generation closure — producing
the three clean structs shown in `golden_aya/types.go`.

Note: even without `--aya`, the `_BTF_EXPORT_*` statics ensure the standalone BTF
entries for V types exist in the ELF (so bpf-linker embeds them). btf2go's
`.rodata` DATASEC vars are not auto-traversed, so `--aya` is still required to
surface them from map context. `BareStruct` is accessible via an explicit
`--type BareStruct` request regardless of `--aya`.

## Toolchain

- Rust `nightly-2026-04-15` (pinned in `rust-toolchain.toml`)
- bpf-linker 0.10.3
- aya-ebpf 0.1 (see `Cargo.toml`)

## Rebuilding the ELF

```sh
cd tests/fixtures/rust-aya/maps-happy
cargo +nightly-2026-04-15 build --target bpfel-unknown-none --release -Z build-std=core
cp target/bpfel-unknown-none/release/maps-happy maps-happy.elf
```

## Regenerating goldens

```sh
go run github.com/danigoland/btf2go/cmd/btf2go generate \
    --elf tests/fixtures/rust-aya/maps-happy/maps-happy.elf \
    --pkg fixture --type BareStruct \
    --out tests/fixtures/rust-aya/maps-happy/golden_no_aya/types.go

go run github.com/danigoland/btf2go/cmd/btf2go generate \
    --elf tests/fixtures/rust-aya/maps-happy/maps-happy.elf \
    --pkg fixture --aya \
    --out tests/fixtures/rust-aya/maps-happy/golden_aya/types.go
```
