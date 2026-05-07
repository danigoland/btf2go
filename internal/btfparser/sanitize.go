// Package btfparser handles Phase 1 (load) and Phase 2 (resolve and
// sanitize) of the pipeline. Sanitization here only handles names; the
// type-graph traversal lives in internal/traverse.
package btfparser

import (
	"fmt"
	"strings"
	"unicode"
)

// SanitizeName converts a BTF type or member name into a valid Go
// PascalCase identifier. It strips Rust/C++ namespace separators (::),
// dots, hyphens, and other non-identifier characters, capitalizing the
// next runic letter so MyMod::Event becomes MyModEvent.
//
// Empty names become "_anon" (callers wanting numbered anonymous names
// should use AnonName).
func SanitizeName(s string) string {
	if s == "" {
		return "_anon"
	}
	var out []rune
	upper := true
	for _, r := range s {
		switch {
		case r == ':' || r == '.' || r == '-' || r == '_' || r == '/' || unicode.IsSpace(r):
			upper = true
			continue
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			if upper && unicode.IsLetter(r) {
				r = unicode.ToUpper(r)
			}
			upper = false
			out = append(out, r)
		default:
			// drop other punctuation
			upper = true
		}
	}
	if len(out) == 0 {
		return "_anon"
	}
	if unicode.IsDigit(out[0]) {
		out = append([]rune{'_'}, out...)
	}
	return string(out)
}

// AnonName generates a deterministic name for an anonymous nested
// struct or union: "<Parent><Field>Anon<N>". Either parent or field may
// be empty; if both are, returns "Anon<N>".
func AnonName(parent, field string, n int) string {
	p := SanitizeName(parent)
	f := SanitizeName(field)
	if p == "_anon" {
		p = ""
	}
	if f == "_anon" {
		f = ""
	}
	return fmt.Sprintf("%s%sAnon%d", strings.TrimPrefix(p, ""), strings.TrimPrefix(f, ""), n)
}
