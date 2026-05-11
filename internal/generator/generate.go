// Package generator implements Phase 5: turn a types.GoFile into a
// formatted .go source file.
//
// Implementation note: codegen uses strings.Builder rather than
// text/template. Bitfield Get/Set accessor bodies are mostly bit math
// (shift/mask, conditional sign-extension, per-byte spill placement)
// which is awkward to express in Go's template language. The trade-off
// is that file scaffolding (header, package, imports, Pointer wrapper)
// also lives in the same Go-string code rather than in a template
// file. The templates/file.tmpl file is retained as documentation of
// the file shape only.
package generator

import (
	"fmt"
	"go/format"
	"os"
	"path/filepath"
	"strings"

	"github.com/danigoland/btf2go/internal/types"
)

// Options controls codegen behavior. The Source path and ToolVersion
// are recorded in the generated file's header comment.
type Options struct {
	Source      string
	ToolVersion string

	// SourceName, when non-empty, overrides the value written to the
	// "// Source:" header comment. When empty, the header uses
	// filepath.Base(Source). Set this to a deterministic identifier
	// (e.g. "bpf/my-crate") to keep generated files diff-stable across
	// build hosts.
	SourceName string

	// SharedOut, when non-empty, splits emission: Pointer[T] and any
	// types named in SharedTypes go to this file (via MergeSharedFile);
	// the Generate return value contains only the remaining per-ELF
	// types.
	SharedOut   string
	SharedTypes []string
}

// Generate renders the IR to formatted Go source. On formatter failure
// the unformatted bytes are returned alongside the formatter error so
// the caller may still write them to disk for debugging.
func Generate(f *types.GoFile, opts Options) ([]byte, error) {
	if f == nil {
		return nil, fmt.Errorf("generator: nil GoFile")
	}

	// Resolve the source label once — used for both the shared-file header
	// and the per-ELF render call, so both see the same resolved value.
	resolvedSource := opts.SourceName
	if resolvedSource == "" {
		resolvedSource = filepath.Base(opts.Source)
	}

	if opts.SharedOut != "" {
		sharedSet := make(map[string]bool, len(opts.SharedTypes))
		for _, n := range opts.SharedTypes {
			sharedSet[n] = true
		}

		// Compute the transitive closure: walk struct fields and pull in any
		// referenced generated types (structs, enums, unions) recursively.
		extendSharedSet(sharedSet, f)

		sharedDefs := map[string]string{}
		for _, s := range f.Structs {
			if !sharedSet[s.Name] {
				continue
			}
			body, err := renderStructOnly(s)
			if err != nil {
				return nil, fmt.Errorf("render shared struct %s: %w", s.Name, err)
			}
			sharedDefs[s.Name] = body
		}
		// Emit enums and unions that landed in the closure.
		for _, e := range f.Enums {
			if !sharedSet[e.Name] {
				continue
			}
			body, err := renderEnumOnly(e)
			if err != nil {
				return nil, fmt.Errorf("render shared enum %s: %w", e.Name, err)
			}
			sharedDefs[e.Name] = body
		}
		for _, u := range f.Unions {
			if !sharedSet[u.Name] {
				continue
			}
			body, err := renderUnionOnly(u)
			if err != nil {
				return nil, fmt.Errorf("render shared union %s: %w", u.Name, err)
			}
			sharedDefs[u.Name] = body
		}

		// Unions use unsafe.Pointer in their accessor methods.
		needsUnsafe := false
		for _, u := range f.Unions {
			if sharedSet[u.Name] {
				needsUnsafe = true
				break
			}
		}

		pointerDecl := "type Pointer[T any] uint64\n"
		if err := MergeSharedFile(MergeArgs{
			Path:           opts.SharedOut,
			Package:        f.Package,
			SourcePath:     resolvedSource,
			PointerDecl:    pointerDecl,
			SharedTypeDefs: sharedDefs,
			NeedsUnsafe:    needsUnsafe,
		}); err != nil {
			return nil, err
		}

		// Build a filtered file with shared types removed and Pointer omitted.
		// Note: build NEW slices — do NOT mutate the backing arrays.
		filtered := *f
		filtered.Structs = nil
		for _, s := range f.Structs {
			if sharedSet[s.Name] {
				continue
			}
			filtered.Structs = append(filtered.Structs, s)
		}
		filtered.Enums = nil
		for _, e := range f.Enums {
			if sharedSet[e.Name] {
				continue
			}
			filtered.Enums = append(filtered.Enums, e)
		}
		filtered.Unions = nil
		for _, u := range f.Unions {
			if sharedSet[u.Name] {
				continue
			}
			filtered.Unions = append(filtered.Unions, u)
		}
		filtered.OmitPointer = true
		f = &filtered
	}

	// Ensure render sees the same resolved source label.
	if opts.SourceName == "" {
		opts.SourceName = resolvedSource
	}
	rendered, err := render(f, opts)
	if err != nil {
		return nil, err
	}
	formatted, fErr := format.Source(rendered)
	if fErr != nil {
		// Print the unformatted source to stderr so the developer can
		// debug template/codegen issues. The unformatted bytes are
		// still returned so the caller may write them to disk.
		_, _ = fmt.Fprintln(os.Stderr, string(rendered))
		return rendered, fmt.Errorf("go/format: %w", fErr)
	}
	return formatted, nil
}

// sanitizeHeaderField escapes carriage returns and continues newlines
// with a "// " prefix so that opts.Source / opts.ToolVersion cannot
// break out of the file's leading comment block (preventing a class of
// generated-code injection).
var sanitizeHeaderField = strings.NewReplacer("\r", " ", "\n", "\n// ").Replace

func render(f *types.GoFile, opts Options) ([]byte, error) {
	needsUnsafe := len(f.Unions) > 0
	var sb strings.Builder
	src := opts.SourceName
	if src == "" {
		src = filepath.Base(opts.Source)
	}
	sb.WriteString("// Code generated by btf2go. DO NOT EDIT.\n")
	fmt.Fprintf(&sb, "// Source: %s\n// Tool:   %s\n\n",
		sanitizeHeaderField(src),
		sanitizeHeaderField(opts.ToolVersion))
	fmt.Fprintf(&sb, "package %s\n\n", f.Package)
	if needsUnsafe {
		sb.WriteString("import \"unsafe\"\n\n")
	}
	if !f.OmitPointer {
		sb.WriteString("type Pointer[T any] uint64\n\n")
	}

	for _, e := range f.Enums {
		fmt.Fprintf(&sb, "type %s %s\n\nconst (\n", e.Name, e.Underlying)
		for _, v := range e.Values {
			renderEnumValue(&sb, e, v)
		}
		sb.WriteString(")\n\n")
	}
	for _, u := range f.Unions {
		fmt.Fprintf(&sb, "type %s struct { %s }\n\n", u.Name, u.Storage)
		for _, a := range u.Accessors {
			fmt.Fprintf(&sb, "func (u *%s) As%s() *%s { return (*%s)(unsafe.Pointer(&u._data)) }\n",
				u.Name, a.Name, a.GoType, a.GoType)
			fmt.Fprintf(&sb, "func (u *%s) SetAs%s(v %s) { *(*%s)(unsafe.Pointer(&u._data)) = v }\n\n",
				u.Name, a.Name, a.GoType, a.GoType)
		}
	}
	for _, s := range f.Structs {
		fmt.Fprintf(&sb, "type %s struct {\n", s.Name)
		for _, fld := range s.Fields {
			fmt.Fprintf(&sb, "\t%s %s\n", fld.Name, fld.GoType)
		}
		sb.WriteString("}\n\n")
		for _, bb := range s.Bitfields {
			for _, a := range bb.Accessors {
				renderBitAccessor(&sb, s.Name, bb, a)
			}
		}
	}
	return []byte(sb.String()), nil
}

// renderStructOnly emits one GoStruct as a verbatim declaration block
// suitable for use as a MergeArgs.SharedTypeDefs value. The output
// excludes the package header and the Pointer[T] declaration.
// The returned body is formatted via go/format for consistency with
// the re-parsed bodies returned by parseSharedFile (which are also
// formatted, because MergeSharedFile calls format.Source on the file).
func renderStructOnly(s types.GoStruct) (string, error) {
	tmp := &types.GoFile{
		Package:     "_shared_render",
		OmitPointer: true,
		Structs:     []types.GoStruct{s},
	}
	raw, err := render(tmp, Options{})
	if err != nil {
		return "", err
	}
	// Format the file so the body is whitespace-normalized the same way
	// the shared file will be when it is re-read by parseSharedFile.
	formatted, fErr := format.Source(raw)
	if fErr != nil {
		// Fallback to raw if formatting fails; normalizeBody handles the diff.
		formatted = raw
	}
	// Drop everything up to the first "\ntype " so the package header
	// is stripped.
	idx := strings.Index(string(formatted), "\ntype ")
	if idx < 0 {
		return "", fmt.Errorf("renderStructOnly: no type decl in output for %s", s.Name)
	}
	return string(formatted[idx+1:]), nil
}

// extendSharedSet computes the transitive closure of types reachable from
// the current sharedSet members. It walks struct fields and adds any
// generated struct / enum / union types it finds. Primitives, Pointer[T],
// and types already in sharedSet are skipped. The walk is cycle-aware
// (already-visited names are not re-queued).
func extendSharedSet(sharedSet map[string]bool, f *types.GoFile) {
	// Build indices for O(1) lookup.
	structIdx := make(map[string]types.GoStruct, len(f.Structs))
	for _, s := range f.Structs {
		structIdx[s.Name] = s
	}
	enumIdx := make(map[string]bool, len(f.Enums))
	for _, e := range f.Enums {
		enumIdx[e.Name] = true
	}
	unionIdx := make(map[string]bool, len(f.Unions))
	for _, u := range f.Unions {
		unionIdx[u.Name] = true
	}

	// BFS over user-specified roots.
	queue := make([]string, 0, len(sharedSet))
	for name := range sharedSet {
		queue = append(queue, name)
	}
	for len(queue) > 0 {
		name := queue[0]
		queue = queue[1:]
		s, ok := structIdx[name]
		if !ok {
			// Enums and unions have no field types to walk through.
			continue
		}
		for _, field := range s.Fields {
			refs := referencedGeneratedTypes(field.GoType, structIdx, enumIdx, unionIdx)
			for _, ref := range refs {
				if !sharedSet[ref] {
					sharedSet[ref] = true
					queue = append(queue, ref)
				}
			}
		}
	}
}

// referencedGeneratedTypes parses a GoType string and returns the names of
// any generated (non-primitive) types it references. Handles:
//   - "[N]T"    → strips the array prefix and checks T
//   - "Pointer[T]" → skip (Pointer is always shared; T may be opaque)
//   - "T"        → T if it's a known struct / enum / union
func referencedGeneratedTypes(
	goType string,
	structs map[string]types.GoStruct,
	enums, unions map[string]bool,
) []string {
	// Strip leading array prefix(es): "[N]T" → "T", "[][N]T" → "[N]T" → "T".
	for strings.HasPrefix(goType, "[") {
		idx := strings.Index(goType, "]")
		if idx < 0 {
			break
		}
		goType = goType[idx+1:]
	}
	// Pointer[T] — skip; Pointer itself is always in shared.
	if strings.HasPrefix(goType, "Pointer[") {
		return nil
	}
	if _, ok := structs[goType]; ok {
		return []string{goType}
	}
	if enums[goType] || unions[goType] {
		return []string{goType}
	}
	return nil
}

// renderEnumOnly emits one GoEnum as a verbatim declaration block
// suitable for use as a MergeArgs.SharedTypeDefs value.
func renderEnumOnly(e types.GoEnum) (string, error) {
	tmp := &types.GoFile{
		Package:     "_shared_render",
		OmitPointer: true,
		Enums:       []types.GoEnum{e},
	}
	raw, err := render(tmp, Options{})
	if err != nil {
		return "", err
	}
	formatted, fErr := format.Source(raw)
	if fErr != nil {
		formatted = raw
	}
	idx := strings.Index(string(formatted), "\ntype ")
	if idx < 0 {
		return "", fmt.Errorf("renderEnumOnly: no type decl in output for %s", e.Name)
	}
	return string(formatted[idx+1:]), nil
}

// renderUnionOnly emits one GoUnion as a verbatim declaration block
// suitable for use as a MergeArgs.SharedTypeDefs value.
func renderUnionOnly(u types.GoUnion) (string, error) {
	tmp := &types.GoFile{
		Package:     "_shared_render",
		OmitPointer: true,
		Unions:      []types.GoUnion{u},
	}
	raw, err := render(tmp, Options{})
	if err != nil {
		return "", err
	}
	formatted, fErr := format.Source(raw)
	if fErr != nil {
		formatted = raw
	}
	idx := strings.Index(string(formatted), "\ntype ")
	if idx < 0 {
		return "", fmt.Errorf("renderUnionOnly: no type decl in output for %s", u.Name)
	}
	return string(formatted[idx+1:]), nil
}

// renderEnumValue formats a single enum const. When the enum is signed,
// the uint64-stored value is reinterpreted as signed (sign-extending
// from the underlying width) so negative constants render as "-1"
// rather than as a huge positive number.
func renderEnumValue(sb *strings.Builder, e types.GoEnum, v types.GoEnumValue) {
	if !e.Signed {
		fmt.Fprintf(sb, "\t%s %s = %d\n", v.Name, e.Name, v.Value)
		return
	}
	var signed int64
	switch e.Underlying {
	case "int8":
		signed = int64(int8(v.Value))
	case "int16":
		signed = int64(int16(v.Value))
	case "int32":
		signed = int64(int32(v.Value))
	default: // int64
		signed = int64(v.Value)
	}
	fmt.Fprintf(sb, "\t%s %s = %d\n", v.Name, e.Name, signed)
}

// renderBitAccessor emits Get/Set methods for a single bitfield.
//
// Layout: bit i of the storage block lives in byte i/8 at bit (i%8).
// Get<Name> reads `span` bytes starting at startByte, ORs them shifted
// into a uint64, shifts down by bitInByte, masks to BitWidth, and (if
// Signed) sign-extends. Set<Name> masks the input value to BitWidth
// bits, then for each affected byte writes (byte((v<<bitInByte)>>i*8))
// after clearing that byte's portion of the field's mask.
//
// Widths up to 64 bits are supported; widths >64 emit a comment-only
// stub (v0.1 limitation).
func renderBitAccessor(sb *strings.Builder, structName string, bb types.GoBitfieldBlock, a types.GoBitAccessor) {
	if a.BitWidth > 64 {
		fmt.Fprintf(sb, "// Get%s/Set%s: bitfield wider than 64 bits is not supported by btf2go v0.1\n\n", a.Name, a.Name)
		return
	}
	bitInByte := a.BitOffset % 8
	// A 64-bit bitfield that doesn't start on a byte boundary spans
	// nine bytes, which can't fit in the uint64 the accessor uses to
	// shift bits around. We emit a stub instead of silently truncating
	// the high bits — see review of v0.1.1.
	if a.BitWidth == 64 && bitInByte != 0 {
		fmt.Fprintf(sb, "// Get%s/Set%s: 64-bit bitfield at non-byte-aligned bit offset %d is not supported by btf2go v0.1\n\n", a.Name, a.Name, a.BitOffset)
		return
	}
	// Go semantics: uint64(1)<<64 == 0, so mask = 0-1 = ^uint64(0) when
	// BitWidth is exactly 64. That is the correct full mask.
	mask := (uint64(1) << a.BitWidth) - 1
	startByte := a.BitOffset / 8
	span := (bitInByte + a.BitWidth + 7) / 8
	if span > 8 {
		span = 8
	}

	// --- Getter ---
	fmt.Fprintf(sb, "func (s *%s) Get%s() %s {\n", structName, a.Name, a.GoType)
	sb.WriteString("\tvar v uint64\n")
	for i := uint32(0); i < span; i++ {
		fmt.Fprintf(sb, "\tv |= uint64(s.%s[%d]) << %d\n", bb.StorageField, startByte+i, i*8)
	}
	fmt.Fprintf(sb, "\tv = (v >> %d) & 0x%x\n", bitInByte, mask)
	if a.Signed && a.BitWidth > 0 && a.BitWidth < 64 {
		// Sign-extend: if the top bit of the field is set, OR in all
		// bits above the field. We use the bitwise complement of the
		// field mask rather than `^uint64(0) << BitWidth`, because the
		// latter is a constant expression that overflows uint64 when
		// the Go compiler folds it for small widths.
		fmt.Fprintf(sb, "\tif v&(1<<%d) != 0 {\n\t\tv |= ^uint64(0x%x)\n\t}\n", a.BitWidth-1, mask)
	}
	fmt.Fprintf(sb, "\treturn %s(v)\n}\n\n", a.GoType)

	// --- Setter ---
	// Unified placement formula: for byte i in [0, span), the bits we
	// want in storage byte (startByte+i) are byte((v<<bitInByte)>>i*8)
	// masked to that byte's portion of the field mask. This covers
	// both the "field starts mid-byte" case (i=0, shift left by
	// bitInByte) and the "spill into higher bytes" case (i>0, shift
	// right after the same left-shift) without an if/else branch.
	fmt.Fprintf(sb, "func (s *%s) Set%s(val %s) {\n", structName, a.Name, a.GoType)
	fmt.Fprintf(sb, "\tv := uint64(val) & 0x%x\n", mask)
	for i := uint32(0); i < span; i++ {
		fmt.Fprintf(sb, "\t{\n")
		// Parenthesize fully so left-to-right same-precedence shifts
		// don't accidentally mask before shifting.
		fmt.Fprintf(sb, "\t\tbyteMask := byte(((uint64(0x%x) << %d) >> %d) & 0xff)\n", mask, bitInByte, i*8)
		fmt.Fprintf(sb, "\t\tnewByte := byte((v << %d) >> %d) & byteMask\n", bitInByte, i*8)
		fmt.Fprintf(sb, "\t\ts.%s[%d] = (s.%s[%d] &^ byteMask) | newByte\n", bb.StorageField, startByte+i, bb.StorageField, startByte+i)
		fmt.Fprintf(sb, "\t}\n")
	}
	sb.WriteString("}\n\n")
}
