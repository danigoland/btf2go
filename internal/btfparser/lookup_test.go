package btfparser

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/cilium/ebpf/btf"
)

// makeSpec builds an in-memory *btf.Spec containing one btf.Struct per name.
func makeSpec(t *testing.T, names ...string) *btf.Spec {
	t.Helper()
	var b btf.Builder
	i32 := &btf.Int{Name: "i32", Size: 4, Encoding: btf.Signed}
	if _, err := b.Add(i32); err != nil {
		t.Fatalf("btf.Builder.Add i32: %v", err)
	}
	for _, n := range names {
		s := &btf.Struct{
			Name:    n,
			Size:    4,
			Members: []btf.Member{{Name: "x", Type: i32, Offset: 0}},
		}
		if _, err := b.Add(s); err != nil {
			t.Fatalf("btf.Builder.Add %q: %v", n, err)
		}
	}
	raw, err := b.Marshal(nil, nil)
	if err != nil {
		t.Fatalf("btf.Builder.Marshal: %v", err)
	}
	spec, err := btf.LoadSpecFromReader(bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("btf.LoadSpecFromReader: %v", err)
	}
	return spec
}

func TestLookupTypeByName(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		spec := makeSpec(t, "firelxc_common::ScaffoldPing")
		got, err := LookupTypeByName(spec, "firelxc_common::ScaffoldPing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.TypeName() != "firelxc_common::ScaffoldPing" {
			t.Errorf("got %q, want %q", got.TypeName(), "firelxc_common::ScaffoldPing")
		}
	})

	t.Run("terminal segment fallback", func(t *testing.T) {
		spec := makeSpec(t, "ScaffoldPing")
		got, err := LookupTypeByName(spec, "firelxc_common::ScaffoldPing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.TypeName() != "ScaffoldPing" {
			t.Errorf("got %q, want %q", got.TypeName(), "ScaffoldPing")
		}
	})

	t.Run("sanitized fallback", func(t *testing.T) {
		spec := makeSpec(t, "FirelxcCommonScaffoldPing")
		got, err := LookupTypeByName(spec, "firelxc_common::ScaffoldPing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.TypeName() != "FirelxcCommonScaffoldPing" {
			t.Errorf("got %q, want %q", got.TypeName(), "FirelxcCommonScaffoldPing")
		}
	})

	t.Run("zero hits returns NotFound", func(t *testing.T) {
		spec := makeSpec(t, "Other")
		_, err := LookupTypeByName(spec, "firelxc_common::ScaffoldPing")
		var nfe *LookupNotFoundError
		if !errors.As(err, &nfe) {
			t.Fatalf("expected *LookupNotFoundError, got %T: %v", err, err)
		}
		if !strings.Contains(nfe.Error(), "firelxc_common::ScaffoldPing") {
			t.Errorf("error message missing identifier: %q", nfe.Error())
		}
	})

	t.Run("ambiguous same-tier returns Ambiguous", func(t *testing.T) {
		spec := makeSpec(t, "a::ScaffoldPing", "b::ScaffoldPing")
		_, err := LookupTypeByName(spec, "ScaffoldPing")
		var ae *LookupAmbiguousError
		if !errors.As(err, &ae) {
			t.Fatalf("expected *LookupAmbiguousError, got %T: %v", err, err)
		}
		if len(ae.Candidates) != 2 {
			t.Errorf("expected 2 candidates, got %d: %v", len(ae.Candidates), ae.Candidates)
		}
	})

	t.Run("exact wins over terminal", func(t *testing.T) {
		spec := makeSpec(t, "firelxc_common::ScaffoldPing", "other::ScaffoldPing")
		got, err := LookupTypeByName(spec, "firelxc_common::ScaffoldPing")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.TypeName() != "firelxc_common::ScaffoldPing" {
			t.Errorf("got %q, want exact match", got.TypeName())
		}
	})

	t.Run("sanitized lookup matches namespaced BTF", func(t *testing.T) {
		spec := makeSpec(t, "firelxc_common::ScaffoldPing")
		got, err := LookupTypeByName(spec, "FirelxcCommonScaffoldPing")
		if err != nil {
			t.Fatalf("err = %v", err)
		}
		if got.TypeName() != "firelxc_common::ScaffoldPing" {
			t.Errorf("got %q", got.TypeName())
		}
	})

	t.Run("nil spec returns error", func(t *testing.T) {
		_, err := LookupTypeByName(nil, "Foo")
		if err == nil {
			t.Fatalf("expected error")
		}
	})
}
