# Generating Go types for Aya eBPF maps

`btf2go generate --aya` unwraps the Rust generic instantiation names that
`aya-ebpf` produces (`HashMap<K, V>`, `LruHashMap<K, V>`, `Array<V>`),
finds the layout-bearing value type in BTF, and emits Go for it.

This page walks through the end-to-end flow.

## Why this needs a flag

`aya-ebpf`'s map types look like this (simplified):

```rust
pub struct HashMap<K, V> {
    def:     UnsafeCell<bpf_map_def>,
    _marker: PhantomData<(K, V)>,
}
```

`V`'s bytes never appear in the wrapper. When you write
`#[map] static M: HashMap<u64, Foo> = …;`, rustc lowers the *wrapper
instantiation* into BTF, but `Foo` itself may not appear as a
standalone BTF entry — there's no concrete bytes-on-disk use of it.

`btf2go generate --aya` works around this by:

1. Decoding the wrapper's mangled name
   (`HashMap_3C_u64_2C__20_Foo_3E_`) to recover the head identifier
   (`HashMap`) and type arguments (`u64`, `Foo`).
2. Looking up `Foo` in the BTF type table with a four-tier fallback
   (exact name → terminal segment → sanitized exact → sanitized
   terminal).
3. Seeding the resolved type as an additional resolution root, after
   which the normal pipeline emits Go for it.

## You still need `_BTF_EXPORT_*` (for now)

For `btf2go generate --aya` to find `Foo`, `Foo` has to be in BTF somewhere.
The cheapest way is a `#[no_mangle]` static:

```rust
#[no_mangle]
static _BTF_EXPORT_FOO: Foo = Foo { /* zero-init fields */ };
```

`#[no_mangle]` forces rustc to emit a concrete static, and a concrete
static brings its type into BTF as a standalone entry.

Without this, you get:

```text
Error: aya bridge: type "Foo" referenced by HashMap<...> not resolvable:
type "Foo" not found (tried: exact="Foo", terminal="Foo",
sanitized-exact="Foo")
```

Future versions may ship a helper crate that emits these statics
automatically; for now it's a one-line workaround per V type.

## Default bridge table

`--aya` recognizes three wrappers out of the box:

| Wrapper      | Arity | Layout-bearing position(s) |
|---           |---    |---                          |
| `HashMap`    | 2     | 1 (V)                       |
| `LruHashMap` | 2     | 1 (V)                       |
| `Array`      | 1     | 0 (V)                       |

K is not layout-bearing by default — it's almost always a primitive.
Pass `--type` explicitly if you have a struct-typed K.

## Extending the table: `--aya-bridge`

If you have a custom wrapper or want V-only producers like `RingBuf`,
pass `--aya-bridge Name=arity:positions`. Examples:

```sh
btf2go generate --elf x.elf --pkg p --out y \
  --aya \
  --aya-bridge RingBuf=1:0 \
  --aya-bridge PerfEventArray=1:0
```

`Name=2:0,1` marks both positions as layout-bearing.

`--aya-bridge` implies `--aya`. Overrides replace default entries with
the same head identifier.

## Real example: FireLXC

```sh
btf2go generate \
  --elf target/bpfel-unknown-none/release/firelxc-lsm-ebpf \
  --pkg bpfgen \
  --out orchestrator/internal/bpfgen/lsm_types.go \
  --aya
```

The Rust source has:

```rust
#[map]
static SCAFFOLD_LSM: HashMap<u64, ScaffoldPing> =
    HashMap::with_max_entries(1, 0);

#[no_mangle]
static _BTF_EXPORT_SCAFFOLD_PING: ScaffoldPing =
    ScaffoldPing { timestamp_ns: 0, pid: 0, _pad: 0 };
```

Output `lsm_types.go` contains `type ScaffoldPing struct { … }` with
the correct field layout.

## Multi-ELF dedup

When the same Go package consumes types from two ELFs, use
`--shared-out` + `--shared-type` to route shared types to one file:

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

Without `--shared-type`, two ELFs emitting the same type produce a
`go vet` duplicate-declaration error — the fix is to add
`--shared-type Name` to both invocations.

## Reproducible output across hosts

Pass `--source-name <str>` to override the `// Source: <path>` header
comment with a deterministic identifier:

```sh
btf2go generate --elf $(pwd)/target/.../my-bpf --pkg bpfgen \
  --aya \
  --source-name bpf/my-bpf \
  --out internal/bpfgen/types.go
```

Without `--source-name`, the header defaults to the ELF basename
(`my-bpf` in this case), which is also deterministic across hosts.
This matters when the generated file is committed and PR-gated for
no-drift: absolute paths would otherwise produce a diff on every
re-generate from a different working directory.
