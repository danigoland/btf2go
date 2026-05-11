package generator

import (
	"fmt"
	"go/format"
	"os"
	"regexp"
	"sort"
	"strings"
)

// typeHeaderRE matches the start of a top-level Go type declaration.
var typeHeaderRE = regexp.MustCompile(`^type ([A-Z][A-Za-z0-9_]*)\b`)

// methodLineRE matches a `func (recv *TypeName) MethodName(...)` line.
// Capture group 1 is the receiver type name.
var methodLineRE = regexp.MustCompile(`^func \(\w+ \*(\w+)\)`)

// ScanSharedFile returns the set of top-level type names declared in
// the given file. A missing file returns an empty set with no error.
func ScanSharedFile(path string) (map[string]bool, error) {
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return map[string]bool{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		if m := typeHeaderRE.FindStringSubmatch(line); m != nil {
			result[m[1]] = true
		}
	}
	return result, nil
}

// MergeArgs configures one MergeSharedFile invocation.
type MergeArgs struct {
	Path           string            // absolute path of the shared file
	Package        string            // Go package name (must be valid identifier)
	SourcePath     string            // ELF path contributing this run (for header log)
	PointerDecl    string            // verbatim `type Pointer[T any] uint64\n` block
	SharedTypeDefs map[string]string // typeName -> verbatim decl block (each ending in \n)
}

// MergeSharedFile writes or updates a shared declarations file.
func MergeSharedFile(args MergeArgs) error {
	if args.Path == "" {
		return fmt.Errorf("shared merge: Path must not be empty")
	}
	if args.Package == "" {
		return fmt.Errorf("shared merge: Package must not be empty")
	}

	existing, sources, err := parseSharedFile(args.Path)
	if err != nil {
		return err
	}

	merged := make(map[string]string, len(existing))
	for k, v := range existing {
		merged[k] = v
	}

	// Handle Pointer decl as a special SharedTypeDef entry.
	if args.PointerDecl != "" {
		if old, ok := merged["Pointer"]; ok {
			if normalizeBody(old) != normalizeBody(args.PointerDecl) {
				return fmt.Errorf("shared merge: shape mismatch for type %q between %s and prior runs", "Pointer", args.SourcePath)
			}
		} else {
			merged["Pointer"] = args.PointerDecl
		}
	}

	for name, body := range args.SharedTypeDefs {
		if old, ok := merged[name]; ok {
			if normalizeBody(old) != normalizeBody(body) {
				return fmt.Errorf("shared merge: shape mismatch for type %q between %s and prior runs", name, args.SourcePath)
			}
		} else {
			merged[name] = body
		}
	}

	if !contains(sources, args.SourcePath) {
		sources = append(sources, args.SourcePath)
	}

	rendered := renderSharedFile(args.Package, sources, merged)
	formatted, err := format.Source([]byte(rendered))
	if err != nil {
		return fmt.Errorf("shared merge: go/format: %w (source: %s)", err, rendered)
	}
	return os.WriteFile(args.Path, formatted, 0o644)
}

// reRunAtSuffix matches the legacy "  (run at <timestamp>)" suffix present in
// shared files produced by btf2go v0.4.x. Used for backward-compatible parsing.
var reRunAtSuffix = regexp.MustCompile(`\s+\(run at [^)]+\)\s*$`)

// parseSharedFile reads an existing shared file and returns the type
// bodies and source paths recorded in it. A missing file is not an error.
//
// Source extraction is backward-compatible:
//   - Legacy format (v0.4): "//   path  (run at <RFC3339>)" — strips the suffix
//   - New format (v0.5+):   "//   path" — taken as-is
//
// The parser tracks an inSourcesBlock flag so only lines in the
// "Sources contributing to this file:" header block are treated as
// source-path entries; regular comment lines elsewhere are ignored.
func parseSharedFile(path string) (bodies map[string]string, sources []string, err error) {
	data, readErr := os.ReadFile(path)
	if os.IsNotExist(readErr) {
		return map[string]string{}, nil, nil
	}
	if readErr != nil {
		return nil, nil, readErr
	}

	bodies = map[string]string{}
	lines := strings.Split(string(data), "\n")

	inSourcesBlock := false // true while we're inside the "Sources contributing" block

	for i := 0; i < len(lines); i++ {
		line := lines[i]

		if strings.HasPrefix(line, "//") {
			trimmed := strings.TrimSpace(strings.TrimPrefix(line, "//"))

			// Detect the start of the sources block.
			if strings.HasPrefix(trimmed, "Sources contributing to this file") {
				inSourcesBlock = true
				continue
			}

			if inSourcesBlock {
				// Legacy format: "path  (run at ...)"
				if idx := strings.Index(trimmed, "  (run at "); idx >= 0 {
					src := strings.TrimSpace(trimmed[:idx])
					if src != "" {
						sources = append(sources, src)
					}
					continue
				}
				// New format: "path" (no timestamp suffix)
				// Strip any legacy suffix just in case, then treat remainder as path.
				src := strings.TrimSpace(reRunAtSuffix.ReplaceAllString(trimmed, ""))
				if src != "" {
					sources = append(sources, src)
				}
				continue
			}
			continue
		}

		// A non-comment, non-blank line ends the sources block.
		if inSourcesBlock && strings.TrimSpace(line) != "" {
			inSourcesBlock = false
		}

		m := typeHeaderRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		name := m[1]

		// Single-line decl (no opening brace) or a generic decl ending without {
		if !strings.HasSuffix(strings.TrimSpace(line), "{") {
			bodies[name] = line + "\n"
			continue
		}

		// Multi-line: scan until matching closing brace.
		var sb strings.Builder
		sb.WriteString(line)
		sb.WriteByte('\n')
		depth := strings.Count(line, "{") - strings.Count(line, "}")
		for i++; i < len(lines) && depth > 0; i++ {
			l := lines[i]
			sb.WriteString(l)
			sb.WriteByte('\n')
			depth += strings.Count(l, "{") - strings.Count(l, "}")
		}
		i-- // back up since outer loop will increment

		// After the struct's closing brace, consume any `func (recv *Name) ...`
		// method blocks whose receiver matches this type name. These are the
		// bitfield Get/Set accessors that must travel with the type into shared.
		for j := i + 1; j < len(lines); {
			peek := lines[j]
			// Skip blank lines between method blocks.
			if strings.TrimSpace(peek) == "" {
				j++
				continue
			}
			// Check for a method whose receiver is this type.
			if mm := methodLineRE.FindStringSubmatch(peek); mm != nil && mm[1] == name {
				// Capture the method body (brace-balanced).
				sb.WriteByte('\n')
				sb.WriteString(peek)
				sb.WriteByte('\n')
				methodDepth := strings.Count(peek, "{") - strings.Count(peek, "}")
				j++
				for j < len(lines) && methodDepth > 0 {
					l := lines[j]
					sb.WriteString(l)
					sb.WriteByte('\n')
					methodDepth += strings.Count(l, "{") - strings.Count(l, "}")
					j++
				}
				i = j - 1 // advance outer index past the consumed method
				continue
			}
			// Non-blank, non-method line — stop consuming.
			break
		}

		bodies[name] = sb.String()
	}

	return bodies, sources, nil
}

// renderSharedFile produces the text content of a shared declarations file.
// Source paths are listed without timestamps for deterministic output.
func renderSharedFile(pkg string, sources []string, decls map[string]string) string {
	var sb strings.Builder

	sb.WriteString("// Code generated by btf2go. DO NOT EDIT.\n")
	sb.WriteString("// Shared declarations across multiple ELFs.\n")
	sb.WriteString("//\n")
	sb.WriteString("// Sources contributing to this file:\n")
	for _, src := range sources {
		sb.WriteString("//   ")
		sb.WriteString(sanitizeHeaderAtom(src))
		sb.WriteByte('\n')
	}
	sb.WriteString("package ")
	sb.WriteString(sanitizeHeaderAtom(pkg))
	sb.WriteString("\n\n")

	// Pointer first, then remaining names alphabetically.
	var names []string
	for name := range decls {
		if name != "Pointer" {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	if body, ok := decls["Pointer"]; ok {
		sb.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}

	for _, name := range names {
		body := decls[name]
		sb.WriteString(body)
		if !strings.HasSuffix(body, "\n") {
			sb.WriteByte('\n')
		}
		sb.WriteByte('\n')
	}

	return sb.String()
}

// normalizeBody collapses whitespace runs for shape comparison.
func normalizeBody(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// sanitizeHeaderAtom strips carriage returns and newlines from a string
// used inside a generated-file header comment, preventing header injection.
func sanitizeHeaderAtom(s string) string {
	s = strings.ReplaceAll(s, "\r", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	return strings.TrimSpace(s)
}

// contains reports whether xs contains x.
func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
