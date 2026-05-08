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
