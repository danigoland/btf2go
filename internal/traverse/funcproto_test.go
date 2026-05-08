package traverse

import (
	"strings"
	"testing"

	"github.com/cilium/ebpf/btf"

	"github.com/danigoland/btf2go/internal/types"
)

// TestFuncProtoDirectDegradesToUintptr tests that a bare *btf.FuncProto passed
// to declare() returns "uintptr" without error instead of crashing with
// "unsupported BTF type: *btf.FuncProto".
func TestFuncProtoDirectDegradesToUintptr(t *testing.T) {
	b := &builder{
		out:   &types.GoFile{Package: "testpkg"},
		named: map[btf.Type]string{},
	}
	fp := &btf.FuncProto{Return: &btf.Void{}}
	got, err := b.declare(fp, "")
	if err != nil {
		t.Fatalf("declare(*btf.FuncProto): unexpected error: %v", err)
	}
	if got != "uintptr" {
		t.Errorf("declare(*btf.FuncProto): got %q, want %q", got, "uintptr")
	}
}

// TestFuncDirectDegradesToUintptr tests that a bare *btf.Func passed
// to declare() returns "uintptr" without error.
func TestFuncDirectDegradesToUintptr(t *testing.T) {
	b := &builder{
		out:   &types.GoFile{Package: "testpkg"},
		named: map[btf.Type]string{},
	}
	fn := &btf.Func{Name: "my_func", Type: &btf.FuncProto{Return: &btf.Void{}}}
	got, err := b.declare(fn, "")
	if err != nil {
		t.Fatalf("declare(*btf.Func): unexpected error: %v", err)
	}
	if got != "uintptr" {
		t.Errorf("declare(*btf.Func): got %q, want %q", got, "uintptr")
	}
}

// TestStructWithFuncProtoPointerField tests the real struct_ops scenario:
// a struct containing a field whose BTF type chain is
// *btf.Pointer{Target: *btf.FuncProto{...}}.
// Build() must succeed and the field's GoType must be "Pointer[uintptr]".
func TestStructWithFuncProtoPointerField(t *testing.T) {
	fp := &btf.FuncProto{
		Return: &btf.Int{Size: 4},
		Params: []btf.FuncParam{
			{Name: "ctx", Type: &btf.Pointer{Target: &btf.Void{}}},
		},
	}
	ptr := &btf.Pointer{Target: fp}
	s := &btf.Struct{
		Name: "my_ops",
		Size: 8,
		Members: []btf.Member{
			{Name: "run", Type: ptr, Offset: 0},
		},
	}

	gf, err := Build("testpkg", []btf.Type{s})
	if err != nil {
		t.Fatalf("Build(): unexpected error: %v", err)
	}
	if len(gf.Structs) != 1 {
		t.Fatalf("expected 1 struct, got %d", len(gf.Structs))
	}
	st := gf.Structs[0]
	if st.Name != "MyOps" {
		t.Errorf("struct name: got %q, want %q", st.Name, "MyOps")
	}
	if len(st.Fields) != 1 {
		t.Fatalf("expected 1 field, got %d", len(st.Fields))
	}
	field := st.Fields[0]
	if field.GoType != "Pointer[uintptr]" {
		t.Errorf("field GoType: got %q, want %q", field.GoType, "Pointer[uintptr]")
	}
	if field.Kind != types.KindPointer {
		t.Errorf("field Kind: got %v, want KindPointer", field.Kind)
	}
	_ = strings.Contains // suppress unused import noise
}
