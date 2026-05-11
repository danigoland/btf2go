package generator

import (
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"path/filepath"
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

// TestGenerateSanitizesHeader confirms that a newline in opts.Source
// or opts.ToolVersion can't break out of the leading comment block —
// the header must remain a single contiguous run of // comments.
func TestGenerateSanitizesHeader(t *testing.T) {
	f := &irtypes.GoFile{Package: "events"}
	src, err := Generate(f, Options{
		Source:      "evil.elf\npackage attacker",
		ToolVersion: "v0\nbreak",
	})
	if err != nil {
		t.Fatalf("generate: %v\n%s", err, src)
	}
	// The sanitizer continues newlines as "\n// " so the injected
	// "package attacker" line still starts with "//" and can't open
	// a real package directive.
	if strings.Contains(string(src), "\npackage attacker") {
		t.Fatalf("source-line newline injection broke out of comment:\n%s", src)
	}
	// Same guard for ToolVersion — its second line ("break") must
	// stay commented.
	if strings.Contains(string(src), "\nbreak") {
		t.Fatalf("tool-version newline injection broke out of comment:\n%s", src)
	}
	if !strings.Contains(string(src), "\n// break") {
		t.Fatalf("expected ToolVersion continuation to remain commented:\n%s", src)
	}
	// And the actual package directive should still be there exactly
	// once and at the right place.
	if strings.Count(string(src), "package events") != 1 {
		t.Fatalf("expected exactly one 'package events' directive:\n%s", src)
	}
}

// TestRenderBitAccessorRefuses64BitMisaligned confirms that a 64-bit
// bitfield with a non-zero bit-in-byte offset (which would span 9
// bytes) emits an unsupported stub instead of silently truncating.
func TestRenderBitAccessorRefuses64BitMisaligned(t *testing.T) {
	f := &irtypes.GoFile{
		Package: "events",
		Structs: []irtypes.GoStruct{{
			Name: "S", Size: 16,
			Fields: []irtypes.GoField{
				{Name: "_bf0", Kind: irtypes.KindRawBytes, GoType: "[9]byte", Offset: 0, Size: 9},
				{Name: "_pad0", Kind: irtypes.KindRawBytes, GoType: "[7]byte", Offset: 9, Size: 7, IsPad: true},
			},
			Bitfields: []irtypes.GoBitfieldBlock{{
				StorageField: "_bf0", StorageSize: 9,
				Accessors: []irtypes.GoBitAccessor{
					{Name: "Wide", BitOffset: 4, BitWidth: 64, GoType: "uint64"},
				},
			}},
		}},
	}
	src, err := Generate(f, Options{Source: "x", ToolVersion: "v0.1.x-test"})
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	if !strings.Contains(string(src), "is not supported by btf2go v0.1") {
		t.Fatalf("expected unsupported stub for 64-bit non-aligned bitfield:\n%s", src)
	}
	// Make sure no Get/Set body was actually emitted.
	if strings.Contains(string(src), "func (s *S) GetWide()") {
		t.Fatalf("emitted GetWide body when it should have been stubbed:\n%s", src)
	}
}

func TestGenerate_SharedOut_PointerOnly(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared.go")

	f := &irtypes.GoFile{
		Package: "bpfgen",
		Structs: []irtypes.GoStruct{
			{Name: "Foo", Fields: []irtypes.GoField{{Name: "X", GoType: "uint32"}}},
		},
	}
	out, err := Generate(f, Options{
		Source:    "/elf/lsm.elf",
		SharedOut: shared,
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(string(out), "type Pointer[T any]") {
		t.Errorf("per-ELF output should NOT contain Pointer decl: %s", out)
	}
	data, _ := os.ReadFile(shared)
	if !strings.Contains(string(data), "type Pointer[T any] uint64") {
		t.Errorf("shared file missing Pointer decl: %s", data)
	}
	if strings.Contains(string(data), "type Foo struct") {
		t.Errorf("shared file should NOT contain Foo (not in SharedTypes): %s", data)
	}
	if !strings.Contains(string(out), "type Foo struct") {
		t.Errorf("per-ELF output missing Foo: %s", out)
	}
}

func TestGenerate_SharedOut_SharedTypes(t *testing.T) {
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared.go")

	f := &irtypes.GoFile{
		Package: "bpfgen",
		Structs: []irtypes.GoStruct{
			{Name: "BinaryIdentity", Fields: []irtypes.GoField{{Name: "Inode", GoType: "uint64"}}},
			{Name: "Local", Fields: []irtypes.GoField{{Name: "X", GoType: "uint32"}}},
		},
	}
	out, err := Generate(f, Options{
		Source:      "/elf/lsm.elf",
		SharedOut:   shared,
		SharedTypes: []string{"BinaryIdentity"},
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(string(out), "type BinaryIdentity struct") {
		t.Errorf("per-ELF should NOT contain BinaryIdentity: %s", out)
	}
	if !strings.Contains(string(out), "type Local struct") {
		t.Errorf("per-ELF missing Local: %s", out)
	}
	data, _ := os.ReadFile(shared)
	if !strings.Contains(string(data), "type BinaryIdentity struct") {
		t.Errorf("shared missing BinaryIdentity: %s", data)
	}
}

func TestGenerate_SharedOut_TransitiveClosure(t *testing.T) {
	// Gap 9: --shared-type Outer should pull Inner into shared automatically.
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared.go")

	f := &irtypes.GoFile{
		Package: "bpfgen",
		Structs: []irtypes.GoStruct{
			{
				Name: "Outer",
				Fields: []irtypes.GoField{
					{Name: "Inner", Kind: irtypes.KindNamedStruct, GoType: "Inner"},
					{Name: "Count", Kind: irtypes.KindPrimitive, GoType: "uint32"},
				},
			},
			{
				Name: "Inner",
				Fields: []irtypes.GoField{
					{Name: "X", Kind: irtypes.KindPrimitive, GoType: "uint64"},
				},
			},
			{
				Name: "Unrelated",
				Fields: []irtypes.GoField{
					{Name: "Y", Kind: irtypes.KindPrimitive, GoType: "uint8"},
				},
			},
		},
	}

	out, err := Generate(f, Options{
		Source:      "/elf/x",
		SharedOut:   shared,
		SharedTypes: []string{"Outer"},
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	data, _ := os.ReadFile(shared)
	s := string(data)

	// Outer + Inner go to shared (transitive closure).
	if !strings.Contains(s, "type Outer struct") {
		t.Errorf("shared missing Outer: %s", s)
	}
	if !strings.Contains(s, "type Inner struct") {
		t.Errorf("shared missing Inner (transitive): %s", s)
	}
	// Unrelated stays per-ELF.
	if strings.Contains(s, "type Unrelated struct") {
		t.Errorf("shared unexpectedly contains Unrelated: %s", s)
	}
	if !strings.Contains(string(out), "type Unrelated struct") {
		t.Errorf("per-ELF missing Unrelated: %s", out)
	}
	// Inner should NOT be in per-ELF (moved to shared via closure).
	if strings.Contains(string(out), "type Inner struct") {
		t.Errorf("per-ELF unexpectedly contains Inner: %s", out)
	}
}

func TestGenerate_SharedOut_TransitiveClosure_ArrayField(t *testing.T) {
	// Array field [N]T should also include T in the transitive closure.
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared.go")

	f := &irtypes.GoFile{
		Package: "bpfgen",
		Structs: []irtypes.GoStruct{
			{
				Name: "Outer",
				Fields: []irtypes.GoField{
					{Name: "Items", Kind: irtypes.KindArray, GoType: "[16]Inner"},
				},
			},
			{
				Name: "Inner",
				Fields: []irtypes.GoField{
					{Name: "X", Kind: irtypes.KindPrimitive, GoType: "uint64"},
				},
			},
		},
	}

	_, err := Generate(f, Options{
		Source:      "/elf/x",
		SharedOut:   shared,
		SharedTypes: []string{"Outer"},
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	data, _ := os.ReadFile(shared)
	s := string(data)
	if !strings.Contains(s, "type Inner struct") {
		t.Errorf("shared missing Inner (via [16]Inner array field): %s", s)
	}
}

func TestGenerate_SharedOut_TransitiveClosure_Cycle(t *testing.T) {
	// Cycle guard: A references B, B references A via Pointer[A].
	// Pointer[...] is stripped so no infinite loop, and neither should panic.
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared.go")

	f := &irtypes.GoFile{
		Package: "bpfgen",
		Structs: []irtypes.GoStruct{
			{
				Name: "A",
				Fields: []irtypes.GoField{
					{Name: "B", Kind: irtypes.KindNamedStruct, GoType: "B"},
				},
			},
			{
				Name: "B",
				Fields: []irtypes.GoField{
					// Pointer[A] — Pointer wrapper is skipped by closure walk.
					{Name: "BackToA", Kind: irtypes.KindPointer, GoType: "Pointer[A]"},
				},
			},
		},
	}

	_, err := Generate(f, Options{
		Source:      "/elf/x",
		SharedOut:   shared,
		SharedTypes: []string{"A"},
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}

	data, _ := os.ReadFile(shared)
	s := string(data)
	if !strings.Contains(s, "type A struct") {
		t.Errorf("shared missing A: %s", s)
	}
	if !strings.Contains(s, "type B struct") {
		t.Errorf("shared missing B (transitive from A): %s", s)
	}
}

func TestGenerate_SharedOut_BitfieldMethodsTravelWithStruct(t *testing.T) {
	// Gap 10: bitfield Get/Set methods must travel into the shared file
	// alongside the struct declaration. They should NOT remain in per-ELF.
	dir := t.TempDir()
	shared := filepath.Join(dir, "shared.go")

	f := &irtypes.GoFile{
		Package: "bpfgen",
		Structs: []irtypes.GoStruct{
			{
				Name: "Packed",
				Size: 4,
				Fields: []irtypes.GoField{
					{Name: "_bf0", Kind: irtypes.KindRawBytes, GoType: "[4]byte", Offset: 0, Size: 4},
				},
				Bitfields: []irtypes.GoBitfieldBlock{{
					StorageField: "_bf0", StorageSize: 4,
					Accessors: []irtypes.GoBitAccessor{
						{Name: "A", BitOffset: 0, BitWidth: 8, GoType: "uint8"},
					},
				}},
			},
		},
	}

	out, err := Generate(f, Options{
		Source:      "/elf/lsm.elf",
		SharedOut:   shared,
		SharedTypes: []string{"Packed"},
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}

	data, _ := os.ReadFile(shared)
	s := string(data)

	// The struct and its methods must be in shared.
	if !strings.Contains(s, "type Packed struct") {
		t.Errorf("shared missing Packed struct: %s", s)
	}
	if !strings.Contains(s, "func (s *Packed) GetA()") {
		t.Errorf("shared missing GetA method: %s", s)
	}
	if !strings.Contains(s, "func (s *Packed) SetA(") {
		t.Errorf("shared missing SetA method: %s", s)
	}

	// Per-ELF output must NOT contain the struct or its methods.
	perElf := string(out)
	if strings.Contains(perElf, "type Packed struct") {
		t.Errorf("per-ELF unexpectedly contains Packed struct: %s", perElf)
	}
	if strings.Contains(perElf, "func (s *Packed)") {
		t.Errorf("per-ELF unexpectedly contains Packed methods: %s", perElf)
	}

	// A second run (re-merge) must be idempotent — methods must not be
	// dropped or duplicated on re-scan of the shared file.
	_, err = Generate(f, Options{
		Source:      "/elf/xdp.elf",
		SharedOut:   shared,
		SharedTypes: []string{"Packed"},
	})
	if err != nil {
		t.Fatalf("second Generate: %v", err)
	}
	data2, _ := os.ReadFile(shared)
	s2 := string(data2)
	if cnt := strings.Count(s2, "type Packed struct"); cnt != 1 {
		t.Errorf("Packed struct appears %d times after re-run, want 1: %s", cnt, s2)
	}
	if cnt := strings.Count(s2, "func (s *Packed) GetA()"); cnt != 1 {
		t.Errorf("GetA appears %d times after re-run, want 1: %s", cnt, s2)
	}
}

func TestGenerate_SourceHeader_DefaultBasename(t *testing.T) {
	f := &irtypes.GoFile{
		Package: "fixture",
		Structs: []irtypes.GoStruct{{Name: "Foo"}},
	}
	out, err := Generate(f, Options{
		Source:      "/home/dani/build/firelxc-lsm-ebpf",
		ToolVersion: "v0.4.0",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(string(out), "// Source: firelxc-lsm-ebpf\n") {
		t.Errorf("expected basename in Source line, got:\n%s", out)
	}
	if strings.Contains(string(out), "/home/dani/") {
		t.Errorf("absolute path leaked into Source header: %s", out)
	}
}

func TestGenerate_SourceHeader_Override(t *testing.T) {
	f := &irtypes.GoFile{Package: "fixture", Structs: []irtypes.GoStruct{{Name: "Foo"}}}
	out, err := Generate(f, Options{
		Source:      "/home/dani/build/firelxc-lsm-ebpf",
		SourceName:  "bpf/firelxc-lsm-ebpf",
		ToolVersion: "v0.4.0",
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(string(out), "// Source: bpf/firelxc-lsm-ebpf\n") {
		t.Errorf("override not used: %s", out)
	}
}
