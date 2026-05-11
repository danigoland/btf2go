package btfparser

import "strings"

// DecodedName is the structured form of a Rust/C++-style generic
// instantiation name as it appears in BTF. For example,
// "HashMap_3C_u64_2C__20_Foo_3E_" decodes to
// {Head: "HashMap", Args: []string{"u64", "Foo"}}.
//
// The decoder is intentionally identifier-format-agnostic: it applies
// only the punycode-ish substitution table that rustc + LLVM use to
// turn `<`, `>`, `,`, ` `, and `:` into ASCII-safe BTF identifiers.
// C++ template instantiation names with the same shape decode
// identically.
type DecodedName struct {
	Head string   // pre-`<` head identifier (e.g. "HashMap")
	Args []string // type arguments inside the outermost `<…>`
}

// DecodeMangled returns the decoded form of a generic instantiation
// name. It returns (zero, false) when the name is not a generic
// instantiation (no `_3C_…_3E_` envelope) or is malformed.
//
// Substitution table:
//
//	_3C_ -> <    _3E_ -> >    _2C_ -> ,    _20_ -> ' '    _3A_ -> :
//
// Nested generics are preserved literally in Args (e.g. "Vec<u8>").
// Whitespace from _20_ is trimmed per argument.
func DecodeMangled(name string) (DecodedName, bool) {
	if name == "" {
		return DecodedName{}, false
	}
	decoded := mangleReplacer.Replace(name)
	open := strings.IndexByte(decoded, '<')
	if open <= 0 || !strings.HasSuffix(decoded, ">") {
		return DecodedName{}, false
	}
	head := decoded[:open]
	inner := decoded[open+1 : len(decoded)-1]
	if !validHead(head) {
		return DecodedName{}, false
	}
	args, ok := splitTopLevel(inner)
	if !ok {
		return DecodedName{}, false
	}
	return DecodedName{Head: head, Args: args}, true
}

var mangleReplacer = strings.NewReplacer(
	"_3C_", "<",
	"_3E_", ">",
	"_2C_", ",",
	"_20_", " ",
	"_3A_", ":",
)

// validHead reports whether s is a plausible generic head identifier.
func validHead(s string) bool {
	if s == "" {
		return false
	}
	for i, r := range s {
		switch {
		case r == '_':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		default:
			return false
		}
	}
	return true
}

// splitTopLevel splits a `<…>`-inner string on commas that sit at
// depth 0 (commas inside nested `<…>` are preserved). Returns the
// trimmed argument list and ok=false if angle brackets are unbalanced.
func splitTopLevel(s string) ([]string, bool) {
	var args []string
	depth := 0
	start := 0
	for i, r := range s {
		switch r {
		case '<':
			depth++
		case '>':
			depth--
			if depth < 0 {
				return nil, false
			}
		case ',':
			if depth == 0 {
				arg := strings.TrimSpace(s[start:i])
				if arg == "" {
					return nil, false
				}
				args = append(args, arg)
				start = i + 1
			}
		}
	}
	if depth != 0 {
		return nil, false
	}
	last := strings.TrimSpace(s[start:])
	if last == "" {
		return nil, false
	}
	args = append(args, last)
	return args, true
}
