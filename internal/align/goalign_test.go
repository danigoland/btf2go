package align

import "testing"

func TestGoAlignByGoType(t *testing.T) {
	cases := []struct {
		goType string
		want   uint32
	}{
		{"uint8", 1}, {"int8", 1}, {"bool", 1},
		{"uint16", 2}, {"int16", 2},
		{"uint32", 4}, {"int32", 4},
		{"uint64", 8}, {"int64", 8},
		{"float32", 4}, {"float64", 8}, {"uintptr", 8},
		{"[4]uint8", 1},
		{"[4]uint32", 4},
		{"[16]byte", 1},
		{"[2][3]uint32", 4},
		{"MyStruct", 1},
	}
	for _, c := range cases {
		if got := GoAlign(c.goType); got != c.want {
			t.Errorf("GoAlign(%q) = %d, want %d", c.goType, got, c.want)
		}
	}
}

func TestGoAlignPointer(t *testing.T) {
	if got := GoAlign("Pointer[Foo]"); got != 8 {
		t.Fatalf("Pointer[Foo] should align 8, got %d", got)
	}
}
