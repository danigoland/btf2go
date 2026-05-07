package generator

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"strings"
	"testing"

	irtypes "github.com/danigoland/btf2go/internal/types"
)

func TestGenerateSimpleStruct(t *testing.T) {
	f := &irtypes.GoFile{
		Package: "events",
		Structs: []irtypes.GoStruct{{
			Name: "Foo", Size: 8,
			Fields: []irtypes.GoField{
				{Name: "A", Kind: irtypes.KindPrimitive, GoType: "uint32", Offset: 0, Size: 4},
				{Name: "B", Kind: irtypes.KindPrimitive, GoType: "uint32", Offset: 4, Size: 4},
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
	f := &irtypes.GoFile{
		Package: "events",
		Unions: []irtypes.GoUnion{{
			Name: "U", Size: 4, Storage: "_data [4]byte",
			Accessors: []irtypes.GoUnionAccessor{{Name: "Bar", GoType: "uint32", Size: 4}},
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

// TestGenerateProducesTypeCheckingCode is the real correctness gate
// for the generator. Substring assertions in the other tests catch
// shape regressions; this test catches arithmetic and type errors by
// type-checking the generated source through go/parser + go/types.
//
// The IR exercises a multi-byte signed bitfield run plus a union plus
// a regular field, which collectively touch every emit path.
func TestGenerateProducesTypeCheckingCode(t *testing.T) {
	f := &irtypes.GoFile{
		Package: "events",
		Unions: []irtypes.GoUnion{{
			Name: "Payload", Size: 8, Storage: "_data [8]byte",
			Accessors: []irtypes.GoUnionAccessor{
				{Name: "Raw", GoType: "uint64", Size: 8},
				{Name: "Lo", GoType: "uint32", Size: 4},
			},
		}},
		Structs: []irtypes.GoStruct{{
			Name: "Event", Size: 16,
			Fields: []irtypes.GoField{
				{Name: "_bf0", Kind: irtypes.KindRawBytes, GoType: "[2]byte", Offset: 0, Size: 2},
				{Name: "_pad0", Kind: irtypes.KindRawBytes, GoType: "[2]byte", Offset: 2, Size: 2, IsPad: true},
				{Name: "Pid", Kind: irtypes.KindPrimitive, GoType: "uint32", Offset: 4, Size: 4},
				{Name: "Pay", Kind: irtypes.KindNamedUnion, GoType: "Payload", Offset: 8, Size: 8},
			},
			Bitfields: []irtypes.GoBitfieldBlock{{
				StorageField: "_bf0", StorageSize: 2,
				Accessors: []irtypes.GoBitAccessor{
					// 12-bit signed field that spills across two bytes.
					{Name: "Code", BitOffset: 4, BitWidth: 12, Signed: true, GoType: "int16"},
				},
			}},
		}},
	}
	src, err := Generate(f, Options{Source: "test.elf", ToolVersion: "v0.1.0"})
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, src)
	}

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "events.go", src, parser.AllErrors)
	if err != nil {
		t.Fatalf("parse generated source: %v\n%s", err, src)
	}

	conf := &types.Config{Importer: importer.Default()}
	if _, err := conf.Check("events", fset, []*ast.File{file}, nil); err != nil {
		t.Fatalf("type-check generated source: %v\n%s", err, src)
	}

	for _, want := range []string{
		"GetCode", "SetCode", // bitfield accessors
		"AsRaw", "SetAsRaw", "AsLo", "SetAsLo", // union accessors
	} {
		if !strings.Contains(string(src), want) {
			t.Errorf("missing %q in generated source:\n%s", want, src)
		}
	}
}
