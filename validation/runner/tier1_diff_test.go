package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGoLayouts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	src := `package x

type Foo struct {
	A uint8
	_ [3]byte
	B uint32
}

type Bar struct {
	X uint64
	Y uint64
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseGoLayouts(path)
	if err != nil {
		t.Fatal(err)
	}
	foo := got["Foo"]
	if foo.Size != 8 {
		t.Errorf("Foo size: got %d, want 8", foo.Size)
	}
	if foo.Fields["B"] != 4 {
		t.Errorf("Foo.B offset: got %d, want 4", foo.Fields["B"])
	}
	bar := got["Bar"]
	if bar.Fields["Y"] != 8 || bar.Size != 16 {
		t.Errorf("Bar layout wrong: %+v", bar)
	}
}

// TestParseGoLayoutsPointerGeneric verifies that structs containing
// btf2go's Pointer[T] generic instantiation are NOT silently dropped.
// Pointer[T any] uint64 is always 8 bytes / align 8, regardless of T.
func TestParseGoLayoutsPointerGeneric(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	// btf2go emits Pointer[T any] uint64 as a top-level generic type
	// declaration in every generated file. The AST parser doesn't need
	// it to resolve IndexExpr — but including it keeps the snippet
	// representative of real btf2go output.
	src := `package x

type Pointer[T any] uint64

type WithPointers struct {
	A uint32
	P Pointer[uint32]
	Q Pointer[[1]int32]
	B uint64
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseGoLayouts(path)
	if err != nil {
		t.Fatal(err)
	}
	wp, ok := got["WithPointers"]
	if !ok {
		t.Fatal("WithPointers: not in parsed layouts (struct was silently dropped)")
	}
	// Layout:
	//   offset 0: A  uint32  (4 bytes, align 4)
	//   offset 4: P  Pointer (8 bytes, align 8) → pad 4 bytes first → offset 8
	//   offset 16: Q Pointer (8 bytes, align 8) → no pad → offset 16
	//   offset 24: B uint64  (8 bytes, align 8) → no pad → offset 24
	//   total: 32 bytes
	if wp.Size != 32 {
		t.Errorf("WithPointers size: got %d, want 32", wp.Size)
	}
	wantOffsets := map[string]int64{"A": 0, "P": 8, "Q": 16, "B": 24}
	for field, want := range wantOffsets {
		gotOff, ok := wp.Fields[field]
		if !ok {
			t.Errorf("WithPointers.%s missing in parsed fields", field)
			continue
		}
		if gotOff != want {
			t.Errorf("WithPointers.%s offset: got %d, want %d", field, gotOff, want)
		}
	}
}

func TestParseGoLayoutsToleratesExternalImport(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	src := `package x

import _ "github.com/cilium/ebpf"

type Bpf struct {
	A uint64
	B uint32
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseGoLayouts(path)
	if err != nil {
		t.Fatalf("parseGoLayouts: %v", err)
	}
	if got["Bpf"].Size != 16 {
		t.Errorf("Bpf size: got %d, want 16 (12 with 4 trailing pad to align A=uint64)", got["Bpf"].Size)
	}
	if got["Bpf"].Fields["B"] != 8 {
		t.Errorf("Bpf.B offset: got %d, want 8", got["Bpf"].Fields["B"])
	}
}
