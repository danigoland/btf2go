package btfparser

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/cilium/ebpf/btf"
)

// BridgeSpec describes one entry in the Aya generic-wrapper bridge table.
// Arity is the total type-parameter count; LayoutBearing lists zero-based
// positions whose inner types contribute to closure roots.
type BridgeSpec struct {
	Arity         int
	LayoutBearing []int
}

// BridgeOptions configures BridgeAya. Extra entries override or extend the
// default table on key collision.
type BridgeOptions struct {
	Extra map[string]BridgeSpec
}

// defaultAyaBridge is the built-in bridge table for common Aya map types.
var defaultAyaBridge = map[string]BridgeSpec{
	"HashMap":    {Arity: 2, LayoutBearing: []int{1}}, // V
	"LruHashMap": {Arity: 2, LayoutBearing: []int{1}}, // V
	"Array":      {Arity: 1, LayoutBearing: []int{0}}, // V (sole)
}

// BridgeAya walks all named struct types in spec, decodes generic instantiation
// names via DecodeMangled, looks up wrapper heads in the merged bridge table,
// and returns the resolved layout-bearing inner types (deduplicated, in walk order).
//
// If a wrapper references an inner type that LookupTypeByName cannot resolve,
// the error is wrapped with the wrapper context and returned immediately.
func BridgeAya(spec *btf.Spec, opts BridgeOptions) ([]btf.Type, error) {
	// Merge tables: extras override defaults on collision.
	table := make(map[string]BridgeSpec, len(defaultAyaBridge)+len(opts.Extra))
	for k, v := range defaultAyaBridge {
		table[k] = v
	}
	for k, v := range opts.Extra {
		table[k] = v
	}

	var out []btf.Type
	seen := map[btf.Type]bool{}

	for t, err := range spec.All() {
		if err != nil {
			return nil, fmt.Errorf("iterating BTF spec: %w", err)
		}
		s, ok := t.(*btf.Struct)
		if !ok || s.Name == "" {
			continue
		}
		dec, ok := DecodeMangled(s.Name)
		if !ok {
			continue
		}
		bspec, ok := table[dec.Head]
		if !ok {
			continue
		}
		if len(dec.Args) != bspec.Arity {
			continue
		}
		for _, pos := range bspec.LayoutBearing {
			if pos < 0 || pos >= len(dec.Args) {
				continue
			}
			inner := dec.Args[pos]
			resolved, lerr := LookupTypeByName(spec, inner)
			if lerr != nil {
				return nil, fmt.Errorf("aya bridge: type %q referenced by %s<...> not resolvable: %w", inner, dec.Head, lerr)
			}
			if !seen[resolved] {
				seen[resolved] = true
				out = append(out, resolved)
			}
		}
	}
	return out, nil
}

// ParseBridgeOverride parses one --aya-bridge value of the form
// "Name=arity:positions" (e.g. "MyMap=2:1" or "Both=2:0,1").
// Returns name, parsed spec, or an error on bad format / out-of-range position.
func ParseBridgeOverride(s string) (string, BridgeSpec, error) {
	eqIdx := strings.IndexByte(s, '=')
	if eqIdx < 0 {
		return "", BridgeSpec{}, fmt.Errorf("bridge override %q: expected Name=arity:positions", s)
	}
	name := s[:eqIdx]
	rest := s[eqIdx+1:]

	colonIdx := strings.IndexByte(rest, ':')
	if colonIdx < 0 {
		return "", BridgeSpec{}, fmt.Errorf("bridge override %q: missing ':' after arity", s)
	}
	arityStr := rest[:colonIdx]
	posStr := rest[colonIdx+1:]

	if arityStr == "" {
		return "", BridgeSpec{}, fmt.Errorf("bridge override %q: empty arity", s)
	}
	arity, err := strconv.Atoi(arityStr)
	if err != nil || arity <= 0 {
		return "", BridgeSpec{}, fmt.Errorf("bridge override %q: invalid arity %q", s, arityStr)
	}

	if posStr == "" {
		return "", BridgeSpec{}, fmt.Errorf("bridge override %q: empty positions", s)
	}
	parts := strings.Split(posStr, ",")
	positions := make([]int, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		pos, err := strconv.Atoi(p)
		if err != nil {
			return "", BridgeSpec{}, fmt.Errorf("bridge override %q: invalid position %q", s, p)
		}
		if pos < 0 || pos >= arity {
			return "", BridgeSpec{}, fmt.Errorf("bridge override %q: position %d out of range [0,%d)", s, pos, arity)
		}
		positions = append(positions, pos)
	}

	return name, BridgeSpec{Arity: arity, LayoutBearing: positions}, nil
}
