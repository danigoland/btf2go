package btfparser

import (
	"fmt"
	"sort"
	"strings"

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
			return nil, &TypeNotFoundError{Name: name, Suggestions: suggestNames(spec, name), Cause: err}
		}
		add(t)
	}

	// 2. Map K/V types.
	if opts.IncludeMaps {
		for t, err := range spec.All() {
			if err != nil {
				return nil, fmt.Errorf("iterating BTF spec: %w", err)
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
				// Unwrap typedef/const/volatile/restrict qualifiers
				// before the type assertion so map definitions wrapped
				// in qualifiers are not silently skipped.
				inner, ok := btf.UnderlyingType(v.Type).(*btf.Struct)
				if !ok {
					continue
				}
				for _, m := range inner.Members {
					switch m.Name {
					case "key", "value":
						// Modern BTF maps wrap key/value as pointers, but
						// direct types and typedef/const/volatile/restrict
						// wrappers are also valid per cilium/ebpf. Unwrap
						// once for pointers (since the pointer itself is
						// not the user-visible type) and let the closure
						// walker handle the rest.
						target := btf.UnderlyingType(m.Type)
						if ptr, ok := target.(*btf.Pointer); ok {
							target = ptr.Target
						}
						add(target)
					case "values":
						// __array(values, struct inner_map_t) expands to
						// typeof(inner_map_t) *values[] which BTF encodes as
						// Array{Nelems:0, Elem: Pointer{Target: inner_map_t}}.
						// Unwrap: qualifier-strip → Array → element → Pointer → target.
						arr, ok := btf.UnderlyingType(m.Type).(*btf.Array)
						if !ok {
							continue
						}
						target := btf.UnderlyingType(arr.Type)
						if ptr, ok2 := target.(*btf.Pointer); ok2 {
							target = btf.UnderlyingType(ptr.Target)
						}
						add(target)
					}
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

// TypeNotFoundError is returned by Resolve when an explicit --type
// name doesn't appear in the BTF spec. It carries up to a handful of
// near-match suggestions so the CLI can show a helpful "did you mean"
// hint instead of just failing with the bare cilium/ebpf error.
type TypeNotFoundError struct {
	Name        string
	Suggestions []string
	Cause       error
}

func (e *TypeNotFoundError) Error() string {
	if len(e.Suggestions) == 0 {
		return fmt.Sprintf("--type %q: %v", e.Name, e.Cause)
	}
	return fmt.Sprintf("--type %q not found. Did you mean: %v? (run `btf2go inspect` to list all named types)", e.Name, e.Suggestions)
}

func (e *TypeNotFoundError) Unwrap() error { return e.Cause }

// suggestNames returns up to 3 named struct/union/enum types from
// spec that are likely typos of want, sorted by similarity. Uses a
// cheap two-pass: first prefer case-insensitive substring matches
// (handles users typing "myevent" when the type is "MyEvent" or
// "MyEventT"), then fall back to Levenshtein distance for typos.
func suggestNames(spec *btf.Spec, want string) []string {
	if spec == nil {
		return nil
	}
	wantLower := strings.ToLower(want)

	type candidate struct {
		name     string
		distance int // 0 = exact case-insensitive match; small = similar
	}
	var candidates []candidate

	for t, err := range spec.All() {
		if err != nil {
			break
		}
		var name string
		switch v := t.(type) {
		case *btf.Struct:
			name = v.Name
		case *btf.Union:
			name = v.Name
		case *btf.Enum:
			name = v.Name
		case *btf.Typedef:
			name = v.Name
		default:
			continue
		}
		if name == "" {
			continue
		}
		nameLower := strings.ToLower(name)
		switch {
		case nameLower == wantLower:
			candidates = append(candidates, candidate{name, 0})
		case strings.Contains(nameLower, wantLower) || strings.Contains(wantLower, nameLower):
			candidates = append(candidates, candidate{name, 1})
		default:
			d := levenshtein(wantLower, nameLower)
			// Only suggest if the edit distance is reasonable for the
			// length: at most 2 edits, or 1/3 of the longer string.
			limit := 2
			if l := len(wantLower); l > 6 && l/3 > limit {
				limit = l / 3
			}
			if d <= limit {
				candidates = append(candidates, candidate{name, d + 2})
			}
		}
	}

	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].distance != candidates[j].distance {
			return candidates[i].distance < candidates[j].distance
		}
		return candidates[i].name < candidates[j].name
	})
	// Dedupe by name (multiple BTF kinds can share an identifier:
	// e.g., a struct typedef'd to itself shows up under both Struct
	// and Typedef kinds in the iteration) and cap at 3.
	out := make([]string, 0, 3)
	seen := make(map[string]struct{}, len(candidates))
	for _, c := range candidates {
		if _, ok := seen[c.name]; ok {
			continue
		}
		seen[c.name] = struct{}{}
		out = append(out, c.name)
		if len(out) == 3 {
			break
		}
	}
	return out
}

// levenshtein computes the edit distance between a and b. Iterative
// two-row implementation; O(len(a) * len(b)) time, O(len(b)) space.
func levenshtein(a, b string) int {
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min3(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
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
		return []btf.Type{v.Type, v.Index}
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
	case *btf.TypeTag:
		return []btf.Type{v.Type}
	case *btf.Var:
		return []btf.Type{v.Type}
	}
	return nil
}
