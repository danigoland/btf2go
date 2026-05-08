//go:build linux

package main

import (
	"bytes"
	"fmt"
	"os"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/danigoland/btf2go/validation/runner/wirepkg"
)

// RunTier2_5 round-trips a WireT through a real kernel BPF map. Runs
// only on Linux (build tag) and only when /sys/fs/bpf is mountable.
func RunTier2_5() []Finding {
	if _, err := os.Stat("/sys/fs/bpf"); err != nil {
		return []Finding{{Project: "T2.5-WireT", Status: StatusSkip,
			SkipReason: "/sys/fs/bpf not mounted (no kernel BPF support or not root)"}}
	}
	spec, err := ebpf.LoadCollectionSpec("kernel/wire.elf")
	if err != nil {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: fmt.Sprintf("LoadCollectionSpec: %v", err)}}
	}
	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: fmt.Sprintf("NewCollection: %v", err)}}
	}
	defer coll.Close()
	wireMap := coll.Maps["wire_map"]
	if wireMap == nil {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: "wire_map not found in collection"}}
	}

	src := wirepkg.WireT{
		Kind: 7, Pid: 4242, Ts: 0xDEADBEEF12345678,
	}
	src.SetFlagA(1)
	src.SetFlagB(0)
	src.SetPrio(33)
	for i, b := range []byte("kernel-roundtrip") {
		if i >= len(src.Comm) {
			break
		}
		src.Comm[i] = int8(b)
	}
	src.Pay.SetAsRaw(0xCAFEBABEDEADBEEF)

	key := uint32(1)
	if err := wireMap.Put(key, src); err != nil {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: fmt.Sprintf("map.Put: %v", err)}}
	}

	var got wirepkg.WireT
	if err := wireMap.Lookup(key, &got); err != nil {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: fmt.Sprintf("map.Lookup: %v", err)}}
	}

	srcBytes := unsafe.Slice((*byte)(unsafe.Pointer(&src)), unsafe.Sizeof(src))
	gotBytes := unsafe.Slice((*byte)(unsafe.Pointer(&got)), unsafe.Sizeof(got))
	if !bytes.Equal(srcBytes, gotBytes) {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Summary: "kernel round-trip byte mismatch",
			Detail:  fmt.Sprintf("sent: %x\nrecv: %x", srcBytes, gotBytes)}}
	}

	if got.GetFlagA() != 1 || got.GetFlagB() != 0 || got.GetPrio() != 33 {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: fmt.Sprintf("bitfields: a=%d b=%d prio=%d", got.GetFlagA(), got.GetFlagB(), got.GetPrio())}}
	}
	if *got.Pay.AsRaw() != 0xCAFEBABEDEADBEEF {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: fmt.Sprintf("union round-trip: 0x%x", *got.Pay.AsRaw())}}
	}
	return []Finding{{Project: "T2.5-WireT", Status: StatusPass,
		Summary: fmt.Sprintf("populated/read-back identical (%d bytes)", len(srcBytes))}}
}
