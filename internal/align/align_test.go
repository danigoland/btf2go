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
