package btfparser

import (
	"fmt"

	"github.com/cilium/ebpf/btf"
)

// ResolveOptions controls the closure produced by Resolve.
type ResolveOptions struct {
	ExplicitTypes []string // names from --type
	IncludeMaps   bool     // false when --no-map-types is set
}

// Resolve returns the deduplicated set of BTF types selected for code
// generation, in a stable order suitable for templating.
//
// Selection rule:
//  1. Start with explicit --type names (must resolve, else hard error).
//  2. If IncludeMaps, add the Key and Value types of every map found
//     in the .maps Datasec.
//  3. Take the recursive transitive closure: for every Struct, Union,
//     Array, Pointer, Typedef, Const, Volatile, Restrict in the set,
//     add its referenced types.
//  4. Drop kernel noise: types that come from the base BTF (vmlinux),
//     identified by absence in the user spec when split-loaded. For
//     v0.1 we approximate "user types" by only adding types that
//     appear when iterating spec.All() — kernel types are not added
//     unless explicitly named.
func Resolve(spec *btf.Spec, opts ResolveOptions) ([]btf.Type, error) {
	if spec == nil {
		return nil, fmt.Errorf("resolve: nil BTF spec")
	}
	seen := map[btf.Type]bool{}
	var order []btf.Type

	add := func(t btf.Type) {
		if t == nil || seen[t] {
			return
		}
		seen[t] = true
		order = append(order, t)
	}

	// 1. Explicit types.
	for _, name := range opts.ExplicitTypes {
		t, err := spec.AnyTypeByName(name)
		if err != nil {
			return nil, fmt.Errorf("--type %q: %w", name, err)
		}
		add(t)
	}

	// 2. Map K/V types.
	if opts.IncludeMaps {
		for t, err := range spec.All() {
			if err != nil {
				return nil, err
			}
			ds, ok := t.(*btf.Datasec)
			if !ok || ds.Name != ".maps" {
				continue
			}
			for _, vsi := range ds.Vars {
				v, ok := vsi.Type.(*btf.Var)
				if !ok {
					continue
				}
				inner, ok := v.Type.(*btf.Struct)
				if !ok {
					continue
				}
				for _, m := range inner.Members {
					if m.Name != "key" && m.Name != "value" {
						continue
					}
					// Modern BTF maps wrap key/value as pointers, but
					// direct types and typedef/const/volatile/restrict
					// wrappers are also valid per cilium/ebpf. Unwrap
					// once for pointers (since the pointer itself is
					// not the user-visible type) and let the closure
					// walker handle the rest.
					target := m.Type
					if ptr, ok := target.(*btf.Pointer); ok {
						target = ptr.Target
					}
					add(target)
				}
			}
		}
	}

	// 3. Closure over referenced types.
	for i := 0; i < len(order); i++ {
		for _, dep := range referencedTypes(order[i]) {
			add(dep)
		}
	}

	return order, nil
}

// referencedTypes returns the immediate type dependencies of t (one
// level — the caller iterates to a fixed point).
func referencedTypes(t btf.Type) []btf.Type {
	switch v := t.(type) {
	case *btf.Struct:
		out := make([]btf.Type, 0, len(v.Members))
		for _, m := range v.Members {
			out = append(out, m.Type)
		}
		return out
	case *btf.Union:
		out := make([]btf.Type, 0, len(v.Members))
		for _, m := range v.Members {
			out = append(out, m.Type)
		}
		return out
	case *btf.Array:
		return []btf.Type{v.Type}
	case *btf.Pointer:
		return []btf.Type{v.Target}
	case *btf.Typedef:
		return []btf.Type{v.Type}
	case *btf.Const:
		return []btf.Type{v.Type}
	case *btf.Volatile:
		return []btf.Type{v.Type}
	case *btf.Restrict:
		return []btf.Type{v.Type}
	}
	return nil
}
