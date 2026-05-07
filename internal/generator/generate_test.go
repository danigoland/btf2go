package generator

import (
	"strings"
	"testing"

	"github.com/danigoland/btf2go/internal/types"
)

func TestGenerateSimpleStruct(t *testing.T) {
	f := &types.GoFile{
		Package: "events",
		Structs: []types.GoStruct{{
			Name: "Foo", Size: 8,
			Fields: []types.GoField{
				{Name: "A", Kind: types.KindPrimitive, GoType: "uint32", Offset: 0, Size: 4},
				{Name: "B", Kind: types.KindPrimitive, GoType: "uint32", Offset: 4, Size: 4},
			},
		}},
	}
	out, err := Generate(f, Options{Source: "test.elf", ToolVersion: "v0.1.0"})
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, out)
	}
	s := string(out)
	for _, want := range []string{
		"package events",
		"type Pointer[T any] uint64",
		"type Foo struct",
		"A uint32",
		"B uint32",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in output:\n%s", want, s)
		}
	}
}

func TestGenerateUnionEmitsUnsafeImport(t *testing.T) {
	f := &types.GoFile{
		Package: "events",
		Unions: []types.GoUnion{{
			Name: "U", Size: 4, Storage: "_data [4]byte",
			Accessors: []types.GoUnionAccessor{{Name: "Bar", GoType: "uint32", Size: 4}},
		}},
	}
	out, err := Generate(f, Options{Source: "x", ToolVersion: "v0.1.0"})
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "import \"unsafe\"") {
		t.Fatalf("expected unsafe import:\n%s", out)
	}
	if !strings.Contains(string(out), "AsBar") || !strings.Contains(string(out), "SetAsBar") {
		t.Fatalf("expected union accessors:\n%s", out)
	}
}
