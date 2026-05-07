package types

import "testing"

func TestGoStructTotalFieldSize(t *testing.T) {
	s := GoStruct{
		Name: "Foo",
		Size: 16,
		Fields: []GoField{
			{Name: "A", Kind: KindPrimitive, GoType: "uint32", Offset: 0, Size: 4},
			{Name: "_pad0", Kind: KindRawBytes, GoType: "[4]byte", Offset: 4, Size: 4, IsPad: true},
			{Name: "B", Kind: KindPrimitive, GoType: "uint64", Offset: 8, Size: 8},
		},
	}
	got := s.TotalFieldSize()
	if got != 16 {
		t.Fatalf("want 16, got %d", got)
	}
}
