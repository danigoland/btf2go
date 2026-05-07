package align

import (
	"testing"

	"github.com/danigoland/btf2go/internal/types"
)

func TestApplyInsertsLeadingPadding(t *testing.T) {
	// BTF: u8 at offset 0, u32 at offset 4 → 3 bytes of leading padding
	s := &types.GoStruct{
		Name: "S",
		Size: 8,
		Fields: []types.GoField{
			{Name: "A", Kind: types.KindPrimitive, GoType: "uint8", Offset: 0, Size: 1},
			{Name: "B", Kind: types.KindPrimitive, GoType: "uint32", Offset: 4, Size: 4},
		},
	}
	if err := Apply(s); err != nil {
		t.Fatal(err)
	}
	if len(s.Fields) != 3 {
		t.Fatalf("expected 3 fields after padding, got %d: %+v", len(s.Fields), s.Fields)
	}
	pad := s.Fields[1]
	if !pad.IsPad || pad.GoType != "[3]byte" || pad.Offset != 1 || pad.Size != 3 {
		t.Fatalf("unexpected pad field: %+v", pad)
	}
}

func TestApplyInsertsTrailingPadding(t *testing.T) {
	s := &types.GoStruct{
		Name: "S",
		Size: 8,
		Fields: []types.GoField{
			{Name: "A", Kind: types.KindPrimitive, GoType: "uint32", Offset: 0, Size: 4},
		},
	}
	if err := Apply(s); err != nil {
		t.Fatal(err)
	}
	if len(s.Fields) != 2 {
		t.Fatalf("expected trailing pad, got %+v", s.Fields)
	}
	tail := s.Fields[1]
	if !tail.IsPad || tail.Size != 4 || tail.Offset != 4 {
		t.Fatalf("bad trailing pad: %+v", tail)
	}
}

func TestApplyTightlyPackedNoPadding(t *testing.T) {
	s := &types.GoStruct{
		Name: "S", Size: 8,
		Fields: []types.GoField{
			{Name: "A", Kind: types.KindPrimitive, GoType: "uint32", Offset: 0, Size: 4},
			{Name: "B", Kind: types.KindPrimitive, GoType: "uint32", Offset: 4, Size: 4},
		},
	}
	if err := Apply(s); err != nil {
		t.Fatal(err)
	}
	if len(s.Fields) != 2 {
		t.Fatalf("no padding expected, got %+v", s.Fields)
	}
}

func TestApplyDowngradesPackedUint64(t *testing.T) {
	// uint64 at offset 4 is illegal in Go (would need offset%8==0).
	// Expect: field downgraded to [8]byte, no implicit padding inserted.
	s := &types.GoStruct{
		Name: "Packed", Size: 12,
		Fields: []types.GoField{
			{Name: "A", Kind: types.KindPrimitive, GoType: "uint32", Offset: 0, Size: 4},
			{Name: "B", Kind: types.KindPrimitive, GoType: "uint64", Offset: 4, Size: 8},
		},
	}
	if err := Apply(s); err != nil {
		t.Fatal(err)
	}
	if len(s.Fields) != 2 {
		t.Fatalf("no padding expected, got %d fields", len(s.Fields))
	}
	b := s.Fields[1]
	if b.GoType != "[8]byte" || b.Kind != types.KindRawBytes {
		t.Fatalf("expected B downgraded to [8]byte, got %+v", b)
	}
	if b.Name != "B" {
		t.Fatalf("name should be preserved as B, got %q", b.Name)
	}
}

func TestApplyKeepsAlignedPrimitive(t *testing.T) {
	s := &types.GoStruct{
		Name: "OK", Size: 16,
		Fields: []types.GoField{
			{Name: "A", Kind: types.KindPrimitive, GoType: "uint64", Offset: 0, Size: 8},
			{Name: "B", Kind: types.KindPrimitive, GoType: "uint64", Offset: 8, Size: 8},
		},
	}
	if err := Apply(s); err != nil {
		t.Fatal(err)
	}
	if s.Fields[1].GoType != "uint64" {
		t.Fatalf("aligned uint64 must not be downgraded, got %+v", s.Fields[1])
	}
}

func TestApplyCollapsesBitfieldRun(t *testing.T) {
	// BTF: three bitfields packed into a single byte:
	//   flag_a : 1   bit offset 0
	//   flag_b : 1   bit offset 1
	//   kind   : 6   bit offset 2
	// Total: 8 bits = 1 byte storage. After this, an aligned uint32 at byte 4.
	s := &types.GoStruct{
		Name: "Event", Size: 8,
		Fields: []types.GoField{
			{Name: "flag_a", Kind: types.KindPrimitive, GoType: "uint8", Offset: 0, Size: 1, BitOffset: 0, BitfieldBits: 1},
			{Name: "flag_b", Kind: types.KindPrimitive, GoType: "uint8", Offset: 0, Size: 1, BitOffset: 1, BitfieldBits: 1},
			{Name: "kind", Kind: types.KindPrimitive, GoType: "uint8", Offset: 0, Size: 1, BitOffset: 2, BitfieldBits: 6},
			{Name: "id", Kind: types.KindPrimitive, GoType: "uint32", Offset: 4, Size: 4},
		},
	}
	if err := Apply(s); err != nil {
		t.Fatal(err)
	}
	// Expect: 1 storage field _bf0 [1]byte at offset 0, then 3 bytes pad, then id.
	if len(s.Fields) != 3 {
		t.Fatalf("expected 3 fields (_bf0, _pad0, id), got %d: %+v", len(s.Fields), s.Fields)
	}
	if s.Fields[0].Name != "_bf0" || s.Fields[0].GoType != "[1]byte" || s.Fields[0].Size != 1 {
		t.Fatalf("bad bitfield storage: %+v", s.Fields[0])
	}
	if !s.Fields[1].IsPad || s.Fields[1].Size != 3 {
		t.Fatalf("bad pad after bitfield run: %+v", s.Fields[1])
	}
	if len(s.Bitfields) != 1 {
		t.Fatalf("expected 1 bitfield block, got %d", len(s.Bitfields))
	}
	bb := s.Bitfields[0]
	if bb.StorageField != "_bf0" || bb.StorageSize != 1 || len(bb.Accessors) != 3 {
		t.Fatalf("bad bitfield block: %+v", bb)
	}
	if bb.Accessors[0].Name != "FlagA" || bb.Accessors[0].BitOffset != 0 || bb.Accessors[0].BitWidth != 1 {
		t.Fatalf("bad accessor[0]: %+v", bb.Accessors[0])
	}
	if bb.Accessors[2].Name != "Kind" || bb.Accessors[2].BitOffset != 2 || bb.Accessors[2].BitWidth != 6 {
		t.Fatalf("bad accessor[2]: %+v", bb.Accessors[2])
	}
}

func TestApplyErrorsOnCursorOverflow(t *testing.T) {
	// Two uint64 fields total 16 bytes but Size says 12 — malformed BTF.
	s := &types.GoStruct{
		Name: "Bad", Size: 12,
		Fields: []types.GoField{
			{Name: "A", Kind: types.KindPrimitive, GoType: "uint64", Offset: 0, Size: 8},
			{Name: "B", Kind: types.KindPrimitive, GoType: "uint64", Offset: 8, Size: 8},
		},
	}
	if err := Apply(s); err == nil {
		t.Fatalf("expected error on cursor > size, got nil")
	}
}

func TestApplyErrorsOnBitfieldOverlap(t *testing.T) {
	// Regular u32 at byte 0..4, then a bitfield run starting at byte 2 — overlap.
	s := &types.GoStruct{
		Name: "Overlap", Size: 4,
		Fields: []types.GoField{
			{Name: "A", Kind: types.KindPrimitive, GoType: "uint32", Offset: 0, Size: 4},
			{Name: "bf", Kind: types.KindPrimitive, GoType: "uint8", Offset: 2, Size: 1, BitOffset: 16, BitfieldBits: 1},
		},
	}
	if err := Apply(s); err == nil {
		t.Fatalf("expected overlap error, got nil")
	}
}

func TestApplyErrorsOnRegularFieldOverlap(t *testing.T) {
	// Two non-bitfield uint32 fields claim to start at byte 0 and byte 2
	// — the second overlaps the first. Apply should error.
	s := &types.GoStruct{
		Name: "Overlap2", Size: 8,
		Fields: []types.GoField{
			{Name: "A", Kind: types.KindPrimitive, GoType: "uint32", Offset: 0, Size: 4},
			{Name: "B", Kind: types.KindPrimitive, GoType: "uint32", Offset: 2, Size: 4},
		},
	}
	if err := Apply(s); err == nil {
		t.Fatalf("expected overlap error, got nil")
	}
}
