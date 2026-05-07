// Package types defines the intermediate representation produced by Phase 3
// (btf.Type → IR) and consumed/mutated by Phase 4 (alignment) and Phase 5
// (codegen). The IR has no dependency on github.com/cilium/ebpf/btf.
package types

type Kind int

const (
	KindPrimitive Kind = iota // uintN / intN / bool
	KindArray                 // [N]T
	KindPointer               // Pointer[T]
	KindNamedStruct           // reference to a GoStruct by name
	// KindNamedUnion is intentionally distinct from KindNamedStruct
	// even though Phase 4 currently treats them identically.
	// Reserved so future codegen paths (e.g. emitting accessor
	// helpers for union fields nested inside structs) can branch
	// on it without an IR shape change.
	KindNamedUnion // reference to a GoUnion by name
	KindRawBytes              // [N]byte (used for padding, packed downgrade, bitfield storage)
)

type GoFile struct {
	Package string
	Enums   []GoEnum
	Unions  []GoUnion
	Structs []GoStruct
}

type GoStruct struct {
	Name      string
	Size      uint32
	Fields    []GoField
	Bitfields []GoBitfieldBlock
}

func (s GoStruct) TotalFieldSize() uint32 {
	var total uint32
	for _, f := range s.Fields {
		total += f.Size
	}
	return total
}

// GoField represents a single field within a GoStruct.
//
// BitOffset and BitfieldBits are only meaningful when BitfieldBits > 0.
// Phase 3 sets these on members of a contiguous bitfield run; Phase 4
// (alignment) reads them when collapsing the run into a single storage
// field and clears them as it emits the resulting [N]byte storage field
// plus accessor metadata in GoStruct.Bitfields.
type GoField struct {
	Name         string
	Kind         Kind
	GoType       string
	Offset       uint32
	Size         uint32
	IsPad        bool
	BitOffset    uint32 // bit offset within parent struct (only when BitfieldBits > 0)
	BitfieldBits uint32 // 0 = not a bitfield
}

type GoBitfieldBlock struct {
	StorageField string
	StorageSize  uint32
	Accessors    []GoBitAccessor
}

type GoBitAccessor struct {
	Name      string
	BitOffset uint32
	BitWidth  uint32
	Signed    bool
	GoType    string
}

type GoEnum struct {
	Name string
	// Underlying is the Go integer type of the enum: int32/int64 if
	// Signed, otherwise uint32/uint64.
	Underlying string
	// Signed mirrors btf.Enum.Signed. BTF stores enum values in
	// unsigned wire fields regardless; the Signed flag tells codegen
	// to interpret and render them as signed (e.g., negative values
	// appear as -1 instead of 0xFFFFFFFFFFFFFFFF).
	Signed bool
	Values []GoEnumValue
}

type GoEnumValue struct {
	Name string
	// Value uses uint64 because BTF enum values are uint32/uint64.
	// The codegen renders the value formatted appropriately for the
	// declared underlying Go type (signed or unsigned) at output time.
	Value uint64
}

type GoUnion struct {
	Name      string
	Size      uint32
	Storage   string
	Accessors []GoUnionAccessor
}

type GoUnionAccessor struct {
	Name   string
	GoType string
	Size   uint32
}
