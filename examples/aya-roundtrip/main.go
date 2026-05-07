// Command aya-roundtrip is the Aya/Rust counterpart of
// examples/c-roundtrip. It walks the full pipeline against an ELF
// emitted by rustc-nightly + bpf-linker (i.e., the Aya kernel-side
// toolchain) to prove the language-agnostic claim:
//
//	rustc/Aya .elf  →  btf2go-generated Go struct  →  cilium/ebpf
//
// The fixture is committed under tests/fixtures/rust/ and rebuilt
// locally via `make -C tests/fixtures rust` (requires rustc nightly,
// bpf-linker, and on macOS DYLD_FALLBACK_LIBRARY_PATH set to the
// Homebrew LLVM lib dir).
//
// Run from the repo root:
//
//	go run ./examples/aya-roundtrip
package main

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"

	"github.com/danigoland/btf2go/tests/fixtures/rust/eventspkg"
)

const elfPath = "tests/fixtures/rust/fixture.elf"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	spec, err := ebpf.LoadCollectionSpec(elfPath)
	if err != nil {
		return fmt.Errorf("load CollectionSpec from %s: %w", elfPath, err)
	}
	if spec.Types == nil {
		return fmt.Errorf("no BTF in %s — was bpf-linker invoked with --btf?", elfPath)
	}
	fmt.Printf("✓ loaded Aya/Rust ELF %s\n", elfPath)
	fmt.Printf("  programs: %d, maps: %d\n", len(spec.Programs), len(spec.Maps))

	var btfStruct *btf.Struct
	if err := spec.Types.TypeByName("EventsT", &btfStruct); err != nil {
		return fmt.Errorf("BTF lookup EventsT: %w", err)
	}
	goSize := unsafe.Sizeof(eventspkg.EventsT{})
	fmt.Printf("✓ EventsT in BTF: size=%d, members=%d\n", btfStruct.Size, len(btfStruct.Members))
	fmt.Printf("  generated EventsT: size=%d (Go unsafe.Sizeof)\n", goSize)
	if uint32(goSize) != btfStruct.Size {
		return fmt.Errorf("size mismatch: Go=%d, BTF=%d", goSize, btfStruct.Size)
	}
	fmt.Println("  ✓ sizes match")

	src := eventspkg.EventsT{
		Kind:         9,
		Pid:          0xCAFE,
		FlagsAndPrio: 0xA5,
		Ts:           0xDEADBEEF12345678,
	}
	for i, b := range []byte("rust-aya-bpf") {
		src.Comm[i] = b
	}
	src.Pay.SetAsRaw(0xCAFEBABEDEADBEEF)

	buf := unsafe.Slice((*byte)(unsafe.Pointer(&src)), int(goSize))
	fmt.Printf("✓ marshalled to %d bytes: %x...%x\n", len(buf), buf[:8], buf[len(buf)-8:])

	got := (*eventspkg.EventsT)(unsafe.Pointer(&buf[0]))
	if got.Kind != 9 || got.Pid != 0xCAFE || got.FlagsAndPrio != 0xA5 {
		return fmt.Errorf("primitives round-trip: kind=%d pid=0x%x flags=0x%x",
			got.Kind, got.Pid, got.FlagsAndPrio)
	}
	if got.Ts != 0xDEADBEEF12345678 {
		return fmt.Errorf("ts round-trip: got 0x%x", got.Ts)
	}
	if *got.Pay.AsRaw() != 0xCAFEBABEDEADBEEF {
		return fmt.Errorf("union round-trip: got 0x%x", *got.Pay.AsRaw())
	}
	fmt.Println("  ✓ primitives and union accessors all round-trip")

	fmt.Println()
	fmt.Println("end-to-end demo: Aya/Rust ELF → btf2go → cilium/ebpf integration works.")
	return nil
}
