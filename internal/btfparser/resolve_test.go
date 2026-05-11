package btfparser

import (
	"testing"

	"github.com/cilium/ebpf/btf"
)

// buildArrayOfMapsSpec constructs a synthetic *btf.Spec that mimics the BTF
// produced by clang for a BPF_MAP_TYPE_ARRAY_OF_MAPS declaration using the
// libbpf __array(values, struct inner_map_t) macro.
//
// The resulting .maps Datasec contains a single outer-map struct whose
// "values" member has BTF type Array{Nelems:0, Type: Pointer{Target: inner}}.
//
// Relevant layout:
//
//	struct inner_map_t { __u32 key; int value; };  // simplified stand-in
//
//	struct { ...  __array(values, struct inner_map_t); } btf_outer_map;
//
// __array expands to  typeof(struct inner_map_t) *values[]
// which BTF encodes as Array{Nelems:0, Elem: Pointer{Target: &inner_map_t}}.
func buildArrayOfMapsSpec(t *testing.T) *btf.Spec {
	t.Helper()

	// inner_map_t — the type we want to see in the resolved output
	inner := &btf.Struct{
		Name: "inner_map_t",
		Size: 4,
		Members: []btf.Member{
			{Name: "type", Type: &btf.Int{Name: "unsigned int", Size: 4}},
		},
	}

	// Pointer to inner_map_t  (typeof(struct inner_map_t) *)
	ptrToInner := &btf.Pointer{Target: inner}

	// Flexible array of those pointers  ([0] in BTF == flexible)
	arrOfPtrs := &btf.Array{Nelems: 0, Type: ptrToInner, Index: &btf.Int{Name: "unsigned int", Size: 4}}

	// The outer map struct — only the "values" member matters for this test
	outerStruct := &btf.Struct{
		Name: "btf_outer_map",
		Size: 4,
		Members: []btf.Member{
			{Name: "type", Type: &btf.Int{Name: "unsigned int", Size: 4}},
			{Name: "values", Type: arrOfPtrs},
		},
	}

	// Var wrapping the outer struct (as BTF encodes global variables)
	outerVar := &btf.Var{
		Name:    "btf_outer_map",
		Type:    outerStruct,
		Linkage: btf.GlobalVar,
	}

	// .maps Datasec containing the outer map variable
	mapsDs := &btf.Datasec{
		Name: ".maps",
		Size: 4,
		Vars: []btf.VarSecinfo{
			{Type: outerVar, Offset: 0, Size: 4},
		},
	}

	b, err := btf.NewBuilder([]btf.Type{inner, ptrToInner, arrOfPtrs, outerStruct, outerVar, mapsDs}, nil)
	if err != nil {
		t.Fatalf("btf.NewBuilder: %v", err)
	}
	spec, err := b.Spec()
	if err != nil {
		t.Fatalf("builder.Spec(): %v", err)
	}
	return spec
}

// TestResolveInnerMapViaValuesMemeber verifies that Resolve walks the
// __array(values, ...) member of an ARRAY_OF_MAPS outer map and includes the
// inner map struct in the resolved type set.
//
// BTF chain:  outerStruct.Member{Name:"values", Type: Array{Elem: Pointer{Target: inner_map_t}}}
//
// Before the fix this test FAILS because the member-name guard only admitted
// "key" and "value", skipping "values".
func TestResolveInnerMapViaValuesMember(t *testing.T) {
	spec := buildArrayOfMapsSpec(t)

	types, err := Resolve(spec, ResolveOptions{IncludeMaps: true})
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	found := false
	for _, ty := range types {
		if s, ok := ty.(*btf.Struct); ok && s.Name == "inner_map_t" {
			found = true
			break
		}
	}
	if !found {
		var names []string
		for _, ty := range types {
			if n := ty.TypeName(); n != "" {
				names = append(names, n)
			}
		}
		t.Errorf("inner_map_t not found in resolved types; got: %v", names)
	}
}

func TestResolve_AyaBridge(t *testing.T) {
	spec := makeSpecWithWrapper(t,
		"HashMap_3C_u64_2C__20_Inner_3E_",
		"Inner",
	)
	types, err := Resolve(spec, ResolveOptions{Aya: true})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !containsType(types, "Inner") {
		t.Errorf("Inner missing from closure, got %v", typeNames(types))
	}
}

func TestResolve_AyaOff_NoExpansion(t *testing.T) {
	spec := makeSpecWithWrapper(t,
		"HashMap_3C_u64_2C__20_Inner_3E_",
		"Inner",
	)
	types, err := Resolve(spec, ResolveOptions{
		ExplicitTypes: []string{"HashMap_3C_u64_2C__20_Inner_3E_"},
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if containsType(types, "Inner") {
		t.Errorf("Inner unexpectedly in closure without --aya")
	}
}

func TestResolve_ExplicitType_FallbackChain(t *testing.T) {
	spec := makeSpec(t, "firelxc_common::ScaffoldPing")
	types, err := Resolve(spec, ResolveOptions{
		ExplicitTypes: []string{"ScaffoldPing"},
	})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !containsType(types, "firelxc_common::ScaffoldPing") {
		t.Errorf("type missing, got %v", typeNames(types))
	}
}
