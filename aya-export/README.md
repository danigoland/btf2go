# btf2go-aya-export

Helper macro for [btf2go](https://github.com/danigoland/btf2go)'s `--aya` flag.

When aya-ebpf's `HashMap<K, V>` (or `LruHashMap<K, V>`, `Array<V>`) wraps a
struct `V`, rustc lowers the wrapper's instantiation into BTF but doesn't emit
`V` itself as a standalone entry — `V`'s bytes are inside `PhantomData<(K, V)>`,
which is a ZST. `btf2go --aya` needs `V` in BTF to emit Go for it.

This crate provides a one-liner that forces `V` into BTF:

```rust
use btf2go_aya_export::btf_export;

#[repr(C)]
pub struct Foo {
    pub x: u64,
}

btf_export!(Foo);          // or: btf_export!(Foo, Bar, Baz);
```

The macro expands to a `#[no_mangle] static _BTF_EXPORT_<NAME>: MaybeUninit<T>`
per type. The static's bytes are uninitialized — only the type's presence in
BTF matters.

## Why MaybeUninit

`MaybeUninit<T>::uninit()` is `const fn` (stable since Rust 1.36), so the
static requires no `Default` impl on `T` and no manual zero-initialization.
The `MaybeUninit<T>` wrapper is `#[repr(transparent)]` over `T`, which causes
rustc to emit `T`'s full structure into BTF as a standalone entry.

## With btf2go

Add the crate to your eBPF crate's `Cargo.toml`:

```toml
[dependencies]
# Until crates.io publish lands (planned post-v0.5), use the git URL:
btf2go-aya-export = { git = "https://github.com/danigoland/btf2go", tag = "v0.5.0" }
```

Then annotate each map value type:

```rust
use btf2go_aya_export::btf_export;

#[repr(C)]
pub struct Foo { /* fields */ }

#[map]
static M: HashMap<u64, Foo> = HashMap::with_max_entries(1, 0);

btf_export!(Foo);
```

Then generate:

```sh
btf2go generate --elf my.elf --pkg bpfgen --aya --out types.go
```

`--aya` finds the exported types via the four-tier fallback chain (exact,
terminal segment, sanitized, etc.). See [docs/aya-maps.md](https://github.com/danigoland/btf2go/blob/master/docs/aya-maps.md).

## Without the macro

Without `btf_export!` (or an equivalent manual `#[no_mangle] static`),
`btf2go generate --aya` fails with:

```text
Error: aya bridge: type "Foo" referenced by HashMap<...> not resolvable:
type "Foo" not found
```

## Compatibility

The `btf2go-aya-export` crate itself compiles on stable Rust. Only the eBPF
target (`bpfel-unknown-none`) requires nightly — that is an aya-ebpf
requirement, not ours.
