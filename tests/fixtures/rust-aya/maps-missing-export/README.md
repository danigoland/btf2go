# tests/fixtures/rust-aya/maps-missing-export

Aya eBPF crate with `HashMap<u64, ScaffoldPing>` but NO `_BTF_EXPORT_*`
static. `ScaffoldPing` is referenced only via PhantomData, so rustc
does not emit a standalone BTF entry for it.

Used by `tests/rust_aya_test.go` (Task 14) to verify that
`btf2go generate --aya` fails loud with the Gap 1 diagnostic, rather
than silently producing empty output.

`expected.err` is the verbatim stderr output expected from the failing
generate run.

## Rebuild

```sh
../refresh.sh maps-missing-export
```
