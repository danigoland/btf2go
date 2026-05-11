# Aya + btf2go Quickstart

This walks through using **btf2go** with an **Aya** kernel program end-to-end: write a Rust eBPF program, compile it with `bpf-linker`, generate Go structs from the resulting BTF, and load it with `cilium/ebpf` from a Go userspace.

This is the workflow `bpf2go` doesn't cover — `bpf2go` expects a C source file, while btf2go reads BTF straight out of any pre-built ELF.

## How to produce a BTF-bearing ELF

`btf2go` reads BTF straight out of an `.elf`. The ELF has to actually
contain BTF — without it you get:

```text
Error: ELF has no .BTF section: <path>
```

### Rust + aya

In your eBPF crate's `.cargo/config.toml`:

```toml
[target.bpfel-unknown-none]
rustflags = ["-C", "link-arg=--btf"]
```

Build with the nightly toolchain and `-Z build-std=core` passed on
the *cargo invocation*, NOT in `.cargo/config.toml`:

```sh
cargo +nightly build --target bpfel-unknown-none --release -Z build-std=core
```

⚠️ **Footgun.** In mixed workspaces (host crates + eBPF crates), avoid
setting `[unstable] build-std = ["core"]` globally in `.cargo/config.toml`.
It can force nightly to rebuild `core` for host targets and trigger
`duplicate lang item "sized"` failures. Prefer passing `-Z build-std=core`
on the bpf-target cargo invocation.

### Clang

Compile with `-g`. Clang's `-target bpf` embeds BTF automatically when
debug info is on:

```sh
clang -O2 -g -target bpf -c hello.c -o hello.elf
```

<a id="zig-toolchain"></a>

### Zig

Zig's `bpf` target embeds BTF by default with current Zig releases.
See `tests/fixtures/zig/` for the build recipe in this repo.

### Verifying

```sh
btf2go inspect --elf <path>
```

If the ELF has BTF, you'll see a list of named types. If it doesn't,
you'll get the diagnostic above with toolchain-specific guidance.

## Prerequisites

- Go 1.24+
- Rust nightly (`rustup install nightly`)
- `bpf-linker` (`cargo install bpf-linker`)
- On macOS: Homebrew LLVM 21+ (`brew install llvm`) and `DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/opt/llvm/lib` exported at build time

> **Note (Linux):** `apt install golang` and the official Go tarball both place binaries under `/usr/local/go/bin`, which is not on `$PATH` by default. Add the following to your shell profile (`~/.bashrc`, `~/.zshrc`, etc.) and re-source it before continuing:
>
> ```sh
> export PATH=$PATH:/usr/local/go/bin:$HOME/go/bin
> ```
>
> `/usr/local/go/bin` is the Go toolchain itself; `$HOME/go/bin` is where `go install` writes compiled binaries like `btf2go`.

## 1. Write the kernel-side struct in Rust

```rust
// src/main.rs
#![no_std]
#![no_main]

use aya_ebpf::{macros::tracepoint, programs::TracePointContext};

#[repr(C)]
pub struct Event {
    pub pid: u32,
    pub uid: u32,
    pub ts_ns: u64,
    pub success: bool,
    // 7 bytes implicit pad
    pub comm: [u8; 16],
}

// Anchor the type into BTF.
#[no_mangle]
#[link_section = ".rodata"]
pub static EVENT_ANCHOR: Event = Event {
    pid: 0,
    uid: 0,
    ts_ns: 0,
    success: false,
    comm: [0; 16],
};

#[tracepoint]
pub fn probe(_ctx: TracePointContext) -> u32 { 0 }

#[cfg(not(test))]
#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! { loop {} }
```

```toml
# Cargo.toml
[package]
name = "myprobe"
version = "0.0.0"
edition = "2021"

[dependencies]
aya-ebpf = "0.1"

[[bin]]
name = "myprobe"
path = "src/main.rs"

[profile.release]
debug = 2
opt-level = 3
panic = "abort"
codegen-units = 1
```

```toml
# .cargo/config.toml  (eBPF-only crate — no host crates in this workspace)
[build]
target = "bpfel-unknown-none"

# OK here because this workspace contains only eBPF crates.
# In mixed workspaces (host crates alongside eBPF crates), remove this
# stanza and pass -Z build-std=core on the bpf-target cargo invocation
# instead (see the Footgun note above).
[unstable]
build-std = ["core"]

[target.bpfel-unknown-none]
linker = "bpf-linker"
rustflags = ["-C", "link-arg=--btf"]
```

> **BTF emission requires `aya-build`.** A bare `cargo init` + `cargo add aya-ebpf` project compiles to a `.elf` with no `.BTF` section. To emit BTF, add `aya-build` as a build-dependency and wire it from `build.rs`:
>
> ```toml
> # Cargo.toml (build-time only)
> [build-dependencies]
> aya-build = "0.1"
> ```
>
> ```rust
> // build.rs
> fn main() {
>     aya_build::build().unwrap();
> }
> ```
>
> Without this `build.rs`, `bpf-linker` won't embed BTF even when `--btf` is passed via `rustflags`, and `btf2go inspect` will report an empty or missing `.BTF` section.

## 2. Build the kernel ELF

```sh
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/opt/llvm/lib \
  cargo +nightly build --release --target bpfel-unknown-none -Z build-std=core
```

Output: `target/bpfel-unknown-none/release/myprobe`.

> **Why `--target` and `-Z build-std=core`?** `bpfel-unknown-none` is a Rust tier-3 target with no prebuilt standard-library artifacts. Cargo must compile `core` from source. Without `-Z build-std=core` you will see linker errors (`undefined symbol: rust_eh_personality`, `main`, `__libc_start_main`) because cargo links the host target instead. The `.cargo/config.toml` above also sets these via `[build] target` and `[unstable] build-std`, so the flags are redundant if you always build from the project root — but spelling them out on the command line makes it explicit and avoids surprises when using `--manifest-path` or other non-standard invocations.

Confirm BTF made it in:

```sh
readelf -S target/bpfel-unknown-none/release/myprobe | grep BTF
# Expect: .BTF  .BTF.ext  (plus relocation sections)
```

`readelf` (from binutils) is available on all Linux distributions and is the recommended portable approach. On Debian/Ubuntu systems, the unversioned `llvm-objdump` binary is not installed by default (only the versioned `llvm-objdump-19` is); `readelf -S` avoids that lookup entirely.

If the `.BTF` section is missing, the `--btf` link-arg didn't take effect or the build profile dropped debug info.

## 3. Inspect the BTF graph

```sh
btf2go inspect --elf target/bpfel-unknown-none/release/myprobe
# KIND     NAME    SIZE  MEMBERS
# DATASEC  .rodata 32    1
# STRUCT   Event   32    5
```

Every type btf2go can target shows up here. If the type you want is missing, `bpf-linker` may have stripped it (often happens with types that aren't transitively reachable from a function or anchored via `#[link_section = ".rodata"]`).

## 4. Generate the Go struct

```sh
btf2go generate \
  --elf target/bpfel-unknown-none/release/myprobe \
  --pkg events \
  --out events_gen.go \
  --type Event \
  --no-map-types
```

Result:

```go
// Code generated by btf2go. DO NOT EDIT.
// Source: target/bpfel-unknown-none/release/myprobe
// Tool:   v0.2.0

package events

type Pointer[T any] uint64

type Event struct {
	Pid     uint32
	Uid     uint32
	TsNs    uint64
	Success bool
	_pad0   [7]byte
	Comm    [16]uint8
}
```

Note: the field order, padding, and type widths exactly match what the Rust compiler emitted — including the 7-byte trailing pad after `Success` because the next field would have needed 8-byte alignment if there were one (here it's just struct alignment).

> **Bitfield `Get`/`Set` accessors are C-only.** btf2go emits `Get<Field>` / `Set<Field>` accessor methods only when the source type used C bitfield syntax (`u8 flag : 1;`). Rust has no arbitrary-width integer types, and common Rust bitfield-emulation patterns (e.g. the `bitflags` crate) store values as regular integer fields at the BTF level — no bitfield metadata is recorded. If your kernel struct needs bitfield accessors in the generated Go, write that type in C and include its ELF alongside the Rust one; or accept that Rust-side bitfields surface as raw `[N]byte` storage fields.

## 5. Use it from Go userspace

```go
package main

import (
	"fmt"
	"unsafe"

	"github.com/cilium/ebpf"

	"yourapp/events"
)

func main() {
	spec, err := ebpf.LoadCollectionSpec("myprobe")
	if err != nil {
		panic(err)
	}
	// load + attach the program, then read events from a ringbuf...
	// See the cilium/ebpf docs for the loader path.
	_ = spec

	// Marshal/unmarshal an event the same way you would any
	// fixed-layout Go struct.
	var e events.Event
	e.Pid = 4242
	e.Success = true
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&e)), int(unsafe.Sizeof(e)))
	fmt.Printf("event bytes: %x\n", buf)
}
```

## 6. Re-run as your kernel struct evolves

```sh
//go:generate btf2go generate --elf ../target/bpfel-unknown-none/release/myprobe --pkg events --out events_gen.go --type Event --no-map-types
```

Drop that comment in any Go file and `go generate ./...` will keep `events_gen.go` in sync after every kernel-side `cargo build`.

## Common pitfalls

- **No BTF section**: missing `--btf` link arg, or you built without `debug = 2` in the release profile. Verify with `readelf -S <elf> | grep BTF`.
- **`--type` not found**: run `btf2go inspect` to see what's actually in the BTF graph. Often the type was tree-shaken because nothing referenced it; anchoring it via a `static` in `.rodata` (as above) prevents that.
- **macOS LLVM mismatch**: `bpf-linker` uses an LLVM-proxy crate that needs to find a libLLVM matching the rustc-nightly toolchain. Set `DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/opt/llvm/lib` and Homebrew LLVM 21+ should satisfy it.
- **bpf-linker dlopen warning is non-fatal.** On nightly toolchains, bpf-linker may print `warning: failed to load shared library libLLVM-XX-rust-Y.Z-nightly.so`. This is expected; bpf-linker falls back to its bundled LLVM. The build still produces a correct ELF. If `Finished` appears at the end of the output, the ELF is good regardless of the warning.
- **Big-endian targets**: btf2go is tested on linux/amd64 and linux/arm64 only. Generating on a same-endianness host as your deployment target is required.
- **`asm/types.h: No such file or directory` on Debian/Ubuntu**: the bare `clang -target bpf` sysroot doesn't include `/usr/include/asm/`. Including `linux/bpf.h` from C-side BPF programs will fail with this error. Add `-I/usr/include/$(uname -m)-linux-gnu` to your clang invocation (e.g. `-I/usr/include/x86_64-linux-gnu`), or write your kernel structs with self-contained typedefs that avoid pulling in `linux/bpf.h`.
- **`btf2go generate` emits only `Pointer[T any] uint64`**: if the generated file contains only the `Pointer[T]` declaration and no struct types, your structs are not on the auto-discovered closure (typically because they're indirectly reachable via map values but not top-level Datasec entries). Pass each struct explicitly: `--type EventT --type CpuId`. Run `btf2go inspect --verbose <elf>` to list all available named types and find the right names.

## What btf2go does *not* generate

- `*ebpf.CollectionSpec` loaders — keep using `ebpf.LoadCollectionSpec` directly. That's `bpf2go`'s territory.
- Map handles — same.
- CO-RE relocation logic — generated structs are layout-correct for the kernel they were built against; CO-RE relocations are applied by `cilium/ebpf` at load time, not by btf2go.

The generated file is **types only**. That's intentional — once you have correct structs, the loader path already works.
