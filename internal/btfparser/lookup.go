package btfparser

import (
	"fmt"
	"strings"

	"github.com/cilium/ebpf/btf"
)

// LookupNotFoundError is returned when no tier of the fallback chain matches.
type LookupNotFoundError struct {
	Identifier string
	Tried      []string
}

func (e *LookupNotFoundError) Error() string {
	return fmt.Sprintf("type %q not found (tried: %s)", e.Identifier, strings.Join(e.Tried, ", "))
}

// LookupAmbiguousError is returned when 2+ types match at the same tier.
type LookupAmbiguousError struct {
	Identifier string
	Tier       string
	Candidates []string
}

func (e *LookupAmbiguousError) Error() string {
	return fmt.Sprintf("type %q is ambiguous at tier %q: %v", e.Identifier, e.Tier, e.Candidates)
}

// LookupTypeByName resolves identifier against the BTF spec using a four-tier
// fallback chain:
//  1. exact — TypeName() == identifier
//  2. terminal — terminal segment of TypeName() == terminal segment of identifier
//  3. sanitized-exact — TypeName() == SanitizeName(identifier)
//  4. sanitized-terminal — terminal segment of TypeName() == SanitizeName(terminal segment of identifier)
//
// Consecutive tiers that would produce identical scans are skipped (deduped by
// matchKey = "fn:want"). 0 matches advances to the next tier; 1 match is
// returned; 2+ matches at the same tier yields LookupAmbiguousError.
// After all tiers LookupNotFoundError is returned.
func LookupTypeByName(spec *btf.Spec, identifier string) (btf.Type, error) {
	if spec == nil {
		return nil, fmt.Errorf("type %q lookup failed: nil BTF spec", identifier)
	}
	terminal := identifier
	if idx := strings.LastIndex(identifier, "::"); idx >= 0 {
		terminal = identifier[idx+2:]
	}
	sanitizedExact := SanitizeName(identifier)
	sanitizedTerm := SanitizeName(terminal)

	// matchFull: TypeName() == want
	// matchTerminal: terminal segment of TypeName() == want
	type tierDef struct {
		name    string
		want    string
		matchFn string // "full" or "terminal"
	}
	tiers := []tierDef{
		{"exact", identifier, "full"},
		{"terminal", terminal, "terminal"},
		{"sanitized-exact", sanitizedExact, "full"},
		{"sanitized-terminal", sanitizedTerm, "terminal"},
	}

	seen := map[string]bool{}
	tried := []string{}
	for _, tier := range tiers {
		matchKey := tier.matchFn + ":" + tier.want
		if seen[matchKey] {
			continue
		}
		seen[matchKey] = true
		tried = append(tried, fmt.Sprintf("%s=%q", tier.name, tier.want))

		var matches []btf.Type
		for t, err := range spec.All() {
			if err != nil {
				return nil, fmt.Errorf("iterating BTF spec: %w", err)
			}
			if !isNamedAggregate(t) {
				continue
			}
			name := t.TypeName()
			var hit bool
			switch tier.matchFn {
			case "full":
				hit = name == tier.want
			case "terminal":
				seg := name
				if idx := strings.LastIndex(name, "::"); idx >= 0 {
					seg = name[idx+2:]
				}
				hit = seg == tier.want
			}
			if hit {
				matches = append(matches, t)
			}
		}
		switch len(matches) {
		case 0:
			continue
		case 1:
			return matches[0], nil
		default:
			candidates := make([]string, len(matches))
			for i, m := range matches {
				candidates[i] = m.TypeName()
			}
			return nil, &LookupAmbiguousError{
				Identifier: identifier,
				Tier:       tier.name,
				Candidates: candidates,
			}
		}
	}
	return nil, &LookupNotFoundError{Identifier: identifier, Tried: tried}
}

// isNamedAggregate reports whether t is a named struct, union, enum, or typedef.
func isNamedAggregate(t btf.Type) bool {
	switch v := t.(type) {
	case *btf.Struct:
		return v.Name != ""
	case *btf.Union:
		return v.Name != ""
	case *btf.Enum:
		return v.Name != ""
	case *btf.Typedef:
		return v.Name != ""
	}
	return false
}
