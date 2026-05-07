// Command c-roundtrip is a sanity-check demo that the full pipeline
// works end-to-end against a real ELF:
//
//	clang fixture .elf  →  btf2go-generated Go struct  →  cilium/ebpf
//
// It does NOT load the BPF program into the kernel (the fixture
// program is a no-op anchor and macOS can't load BPF anyway). It
// proves three things:
//
//  1. cilium/ebpf can parse the ELF's CollectionSpec.
//  2. The BTF type "events_t" appears in the spec's Types and
//     resolves to a Struct with the same byte size that the
//     btf2go-generated Go type reports via unsafe.Sizeof.
//  3. A user can populate the generated struct, unsafe-cast it to
//     bytes, and the bitfield accessors round-trip cleanly.
//
// This is the same end-to-end shape that an Aya/Rust kernel + Go
// userspace deployment hits — only the front of the pipe (clang vs
// rustc/bpf-linker) differs, and btf2go doesn't care about that.
//
// Run from the repo root:
//
//	go run ./examples/c-roundtrip
package main

import (
	"fmt"
	"os"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/btf"

	"github.com/danigoland/btf2go/tests/fixtures/c/eventspkg"
)

const elfPath = "tests/fixtures/c/events.elf"

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}

func run() error {
	// 1. Parse the ELF via cilium/ebpf — exactly what a userspace
	//    program does to load a kernel-side BPF program. We don't
	//    actually load it into the kernel here.
	spec, err := ebpf.LoadCollectionSpec(elfPath)
	if err != nil {
		return fmt.Errorf("load CollectionSpec from %s: %w", elfPath, err)
	}
	if spec.Types == nil {
		return fmt.Errorf("no BTF in %s — was the ELF built with -g?", elfPath)
	}
	fmt.Printf("✓ loaded %s\n", elfPath)
	fmt.Printf("  programs: %d, maps: %d\n", len(spec.Programs), len(spec.Maps))

	// 2. Look up the events_t type in the BTF graph and confirm it
	//    matches what btf2go generated.
	var btfStruct *btf.Struct
	if err := spec.Types.TypeByName("events_t", &btfStruct); err != nil {
		return fmt.Errorf("BTF lookup events_t: %w", err)
	}
	goSize := unsafe.Sizeof(eventspkg.EventsT{})
	fmt.Printf("✓ events_t in BTF: size=%d, members=%d\n", btfStruct.Size, len(btfStruct.Members))
	fmt.Printf("  generated EventsT: size=%d (Go unsafe.Sizeof)\n", goSize)
	if uint32(goSize) != btfStruct.Size {
		return fmt.Errorf("size mismatch: Go=%d, BTF=%d", goSize, btfStruct.Size)
	}
	fmt.Println("  ✓ sizes match")

	// 3. Round-trip: populate the generated struct, cast to bytes,
	//    cast back, exercise accessors. This is the shape a real
	//    consumer hits when reading from a ringbuf or perf event.
	src := eventspkg.EventsT{
		Kind: 7,
		Pid:  4242,
		Ts:   0xDEADBEEF12345678,
	}
	src.SetFlagA(1)
	src.SetFlagB(0)
	src.SetPrio(33)
	// Comm is [16]int8 because C `char` is signed on this target;
	// convert byte-by-byte rather than copy().
	for i, b := range []byte("hello-eBPF") {
		src.Comm[i] = int8(b)
	}
	src.Pay.SetAsRaw(0xCAFEBABEDEADBEEF)

	// Cast to byte slice the way a ringbuf reader does.
	buf := unsafe.Slice((*byte)(unsafe.Pointer(&src)), int(goSize))
	fmt.Printf("✓ marshalled to %d bytes: %x...%x\n", len(buf), buf[:8], buf[len(buf)-8:])

	// Cast back through a fresh pointer.
	got := (*eventspkg.EventsT)(unsafe.Pointer(&buf[0]))
	if got.Kind != 7 || got.Pid != 4242 {
		return fmt.Errorf("primitives round-trip: kind=%d pid=%d", got.Kind, got.Pid)
	}
	if got.GetFlagA() != 1 || got.GetFlagB() != 0 || got.GetPrio() != 33 {
		return fmt.Errorf("bitfields round-trip: a=%d b=%d prio=%d",
			got.GetFlagA(), got.GetFlagB(), got.GetPrio())
	}
	if *got.Pay.AsRaw() != 0xCAFEBABEDEADBEEF {
		return fmt.Errorf("union round-trip: got 0x%x", *got.Pay.AsRaw())
	}
	fmt.Println("  ✓ primitives, bitfields, and union accessors all round-trip")

	fmt.Println()
	fmt.Println("end-to-end demo: ELF → btf2go → cilium/ebpf integration works.")
	return nil
}
