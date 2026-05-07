// Package align implements Phase 4: byte-offset alignment, padding
// insertion, packed-primitive downgrade, and bitfield-block collapse.
//
// This package operates purely on the types.GoFile IR. It must not import
// github.com/cilium/ebpf/btf — that is the rule that keeps alignment logic
// unit-testable with hand-crafted IR.
package align

import "strings"

// GoAlign returns the natural alignment in bytes that the Go gc compiler
// will assign to a value of the given Go type expression on linux/amd64
// and linux/arm64. Targets matter because eBPF userspace runs on those
// architectures; this is not a portable answer for all GOOS/GOARCH pairs.
func GoAlign(goType string) uint32 {
	switch {
	case strings.HasPrefix(goType, "Pointer["):
		return 8
	case strings.HasPrefix(goType, "[") && strings.Contains(goType, "]"):
		// [N]T → align of T
		end := strings.Index(goType, "]")
		return GoAlign(goType[end+1:])
	}
	switch goType {
	case "uint8", "int8", "bool", "byte":
		return 1
	case "uint16", "int16":
		return 2
	case "uint32", "int32":
		return 4
	case "uint64", "int64":
		return 8
	case "float32":
		return 4
	case "float64", "uintptr":
		return 8
	}
	// Named struct/union: caller must supply alignment via lookup;
	// default to 1 for now (callers using named types should compute
	// alignment from the struct's max-aligned member).
	return 1
}
