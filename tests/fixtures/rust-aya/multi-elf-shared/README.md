# tests/fixtures/rust-aya/multi-elf-shared

Cargo workspace with two aya eBPF crates (`lsm`, `xdp`) sharing the
`BinaryIdentity` type from a `common` crate. Exercises Gap 2's
`--shared-out` / `--shared-type` workflow.

## Goldens (happy path)

```sh
btf2go generate --elf lsm.elf --pkg fixture --aya \
  --shared-out golden_shared/types.go --shared-type BinaryIdentity \
  --out golden_lsm/types.go

btf2go generate --elf xdp.elf --pkg fixture --aya \
  --shared-out golden_shared/types.go --shared-type BinaryIdentity \
  --out golden_xdp/types.go
```

- `golden_shared/types.go` — `Pointer[T]` + `BinaryIdentity`
- `golden_lsm/types.go`, `golden_xdp/types.go` — per-ELF package header only (no types; all shared types were routed to the shared file)

## Collision path

`expected_collision.err` is the `go vet` output captured when the
second run is executed WITHOUT `--shared-type BinaryIdentity`. v0.4's
deliberate behavior is to surface this as a Go compile error rather
than silently duplicating declarations.

## Rebuild

```sh
cargo +nightly-2026-04-15 build --target bpfel-unknown-none --release -Z build-std=core
cp target/bpfel-unknown-none/release/lsm lsm.elf
cp target/bpfel-unknown-none/release/xdp xdp.elf
```

The workspace-level `[profile.release]` in `Cargo.toml` sets `debug = 2`
which instructs bpf-linker to embed BTF. The per-crate Cargo.toml files
intentionally omit profile sections (workspace profiles take precedence).
