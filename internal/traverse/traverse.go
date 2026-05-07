// Package traverse implements Phase 3: convert a slice of btf.Type
// (produced by btfparser.Resolve) into a types.GoFile IR (without
// alignment — that is Phase 4's job).
package traverse

import (
	"fmt"
	"strings"

	"github.com/cilium/ebpf/btf"
	"github.com/danigoland/btf2go/internal/btfparser"
	"github.com/danigoland/btf2go/internal/types"
)

// Build produces a GoFile from the resolved BTF type set. Anonymous
// structs/unions encountered during traversal are auto-named via
// btfparser.AnonName and added to the output.
func Build(pkg string, in []btf.Type) (*types.GoFile, error) {
	b := &builder{
		out:   &types.GoFile{Package: pkg},
		named: map[btf.Type]string{},
	}
	for _, t := range in {
		if _, err := b.declare(t, ""); err != nil {
			return nil, err
		}
	}
	return b.out, nil
}

type builder struct {
	out   *types.GoFile
	named map[btf.Type]string
	anonN int
}

// declare ensures t has a Go name and is present in the output;
// returns its Go name. parentField is used to disambiguate anonymous
// nested types.
func (b *builder) declare(t btf.Type, parentField string) (string, error) {
	if name, ok := b.named[t]; ok {
		return name, nil
	}
	switch v := t.(type) {
	case *btf.Int:
		return goIntType(v), nil
	case *btf.Float:
		return goFloatType(v), nil
	case *btf.Void:
		return "uintptr", nil
	case *btf.Typedef:
		return b.declare(v.Type, parentField)
	case *btf.TypeTag:
		return b.declare(v.Type, parentField)
	case *btf.Var:
		// Vars are global symbols (e.g., entries in a .rodata
		// Datasec). The user-visible type is the Var's underlying
		// type, not the Var wrapper itself, so unwrap and recurse.
		// This is what makes `--type SOME_RODATA_CONST` work.
		return b.declare(v.Type, parentField)
	case *btf.Enum:
		return b.declareEnum(v)
	case *btf.Array:
		return b.declareArray(v, parentField)
	case *btf.Pointer:
		return b.declarePointer(v, parentField)
	case *btf.Const:
		return b.declare(v.Type, parentField)
	case *btf.Volatile:
		return b.declare(v.Type, parentField)
	case *btf.Restrict:
		return b.declare(v.Type, parentField)
	case *btf.Struct:
		return b.declareStruct(v, parentField)
	case *btf.Union:
		return b.declareUnion(v, parentField)
	}
	return "", fmt.Errorf("unsupported BTF type: %T", t)
}

// goFloatType maps btf.Float to a Go floating-point type by size.
// Sizes outside {4, 8} (e.g., 2-byte half-precision, 16-byte
// long-double / __float128) are vanishingly rare in real BTF and
// fall back to a raw byte array so the layout stays correct.
// Go has no native types for those widths.
func goFloatType(f *btf.Float) string {
	switch f.Size {
	case 4:
		return "float32"
	case 8:
		return "float64"
	}
	return fmt.Sprintf("[%d]byte", f.Size)
}

// goIntType maps btf.Int to a Go primitive. Width must be 1, 2, 4, or 8.
//
// Bool detection: btf.Int can represent a boolean either via
// Encoding == btf.Bool (the explicit BTF Bool kind, used by C _Bool
// through clang) or via Name == "bool" with Size == 1 (the convention
// rustc uses for Rust's bool type — it emits a plain unsigned int and
// only the type name distinguishes it from u8). Layout is identical
// to uint8 either way; we render it as Go bool for source clarity.
func goIntType(i *btf.Int) string {
	if i.Size == 1 && (i.Encoding == btf.Bool || i.Name == "bool") {
		return "bool"
	}
	signed := i.Encoding&btf.Signed != 0
	prefix := "uint"
	if signed {
		prefix = "int"
	}
	switch i.Size {
	case 1:
		return prefix + "8"
	case 2:
		return prefix + "16"
	case 4:
		return prefix + "32"
	case 8:
		return prefix + "64"
	}
	// Non-power-of-two ints are vanishingly rare; fall back to byte array.
	return fmt.Sprintf("[%d]byte", i.Size)
}

func (b *builder) declareEnum(e *btf.Enum) (string, error) {
	if name, exists := b.named[e]; exists {
		return name, nil
	}
	name := btfparser.SanitizeName(e.Name)
	if name == "_anon" {
		b.anonN++
		name = btfparser.AnonName("", "", b.anonN-1)
	}
	prefix := "uint"
	if e.Signed {
		prefix = "int"
	}
	underlying := prefix + "32"
	if e.Size == 8 {
		underlying = prefix + "64"
	}
	g := types.GoEnum{Name: name, Underlying: underlying, Signed: e.Signed}
	for _, val := range e.Values {
		g.Values = append(g.Values, types.GoEnumValue{
			Name:  name + "_" + btfparser.SanitizeName(val.Name),
			Value: val.Value,
		})
	}
	b.out.Enums = append(b.out.Enums, g)
	b.named[e] = name
	return name, nil
}

func (b *builder) declareArray(a *btf.Array, parentField string) (string, error) {
	inner, err := b.declare(a.Type, parentField)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("[%d]%s", a.Nelems, inner), nil
}

func (b *builder) declarePointer(p *btf.Pointer, parentField string) (string, error) {
	if p.Target == nil {
		return "Pointer[uintptr]", nil
	}
	target, err := b.declare(p.Target, parentField)
	if err != nil {
		// Pointer to unknown type — emit Pointer[uintptr] equivalent.
		return "Pointer[uintptr]", nil
	}
	return fmt.Sprintf("Pointer[%s]", target), nil
}

func (b *builder) declareStruct(s *btf.Struct, parentField string) (string, error) {
	if name, exists := b.named[s]; exists {
		return name, nil
	}
	name := btfparser.SanitizeName(s.Name)
	if name == "_anon" {
		b.anonN++
		name = btfparser.AnonName(parentField, "", b.anonN-1)
	}
	// Reserve the name early so a self-referential pointer terminates.
	b.named[s] = name
	g := types.GoStruct{Name: name, Size: s.Size}
	for _, m := range s.Members {
		mname := btfparser.SanitizeName(m.Name)
		if mname == "_anon" {
			b.anonN++
			mname = btfparser.AnonName(name, "", b.anonN-1)
		}
		mtype, err := b.declare(m.Type, mname)
		if err != nil {
			return "", fmt.Errorf("struct %s field %s: %w", name, mname, err)
		}
		f := types.GoField{
			Name:   mname,
			GoType: mtype,
			Offset: uint32(m.Offset) / 8,
			Size:   memberSize(m),
			Kind:   classifyKind(mtype),
		}
		if m.BitfieldSize > 0 {
			f.BitfieldBits = uint32(m.BitfieldSize)
			f.BitOffset = uint32(m.Offset)
		}
		g.Fields = append(g.Fields, f)
	}
	b.out.Structs = append(b.out.Structs, g)
	return name, nil
}

func (b *builder) declareUnion(u *btf.Union, parentField string) (string, error) {
	if name, exists := b.named[u]; exists {
		return name, nil
	}
	name := btfparser.SanitizeName(u.Name)
	if name == "_anon" {
		b.anonN++
		name = btfparser.AnonName(parentField, "", b.anonN-1)
	}
	b.named[u] = name
	g := types.GoUnion{
		Name: name,
		Size: u.Size,
		// Backing storage uses the smallest array of the union's
		// max-aligned primitive that covers the size. This ensures
		// the storage address is aligned for any cast performed by
		// the As<Member>/SetAs<Member> accessors — without it, a
		// standalone union value could be at a 1-byte-aligned
		// address and a *uint64 cast would SIGBUS on strict-
		// alignment architectures (ARM64, RISC-V, MIPS).
		Storage: unionBackingStorage(u.Size, unionAlignment(u)),
	}
	for _, m := range u.Members {
		mname := btfparser.SanitizeName(m.Name)
		if mname == "_anon" {
			b.anonN++
			mname = btfparser.AnonName(name, "", b.anonN-1)
		}
		mtype, err := b.declare(m.Type, mname)
		if err != nil {
			return "", err
		}
		size := uint32(0)
		if sz, err := btf.Sizeof(m.Type); err == nil && sz >= 0 {
			size = uint32(sz)
		}
		g.Accessors = append(g.Accessors, types.GoUnionAccessor{
			Name: mname, GoType: mtype, Size: size,
		})
	}
	b.out.Unions = append(b.out.Unions, g)
	return name, nil
}

// memberSize returns the byte size of m's type as reported by
// btf.Sizeof. Returns 0 when the size is unknown or negative.
func memberSize(m btf.Member) uint32 {
	if sz, err := btf.Sizeof(m.Type); err == nil && sz >= 0 {
		return uint32(sz)
	}
	return 0
}

// unionAlignment returns the maximum natural alignment of any member
// of u. Used to pick a backing-store type for the generated Go union
// that matches the alignment any accessor will need.
func unionAlignment(u *btf.Union) uint32 {
	var maxAlign uint32 = 1
	for _, m := range u.Members {
		if a := btfAlignment(m.Type); a > maxAlign {
			maxAlign = a
		}
	}
	return maxAlign
}

// btfAlignment computes the natural alignment of a BTF type as the
// Go gc compiler would lay it out on linux/amd64 + linux/arm64.
//
// Note on layering: this is NOT a duplicate of internal/align.GoAlign.
// GoAlign operates on rendered Go type strings, which is what Phase 4
// (the alignment pass) consumes. btfAlignment operates on raw
// btf.Type values, which is what Phase 3 (this package) consumes
// when it needs to pick a union backing-store type before any
// rendering has happened. Same rules, different inputs.
func btfAlignment(t btf.Type) uint32 {
	switch v := btf.UnderlyingType(t).(type) {
	case *btf.Int:
		// Power-of-two sizes ≤ 8 use that size as alignment.
		if v.Size == 1 || v.Size == 2 || v.Size == 4 || v.Size == 8 {
			return v.Size
		}
		return 1
	case *btf.Float:
		if v.Size == 4 || v.Size == 8 {
			return v.Size
		}
		return 1
	case *btf.Pointer:
		return 8
	case *btf.Enum:
		if v.Size == 1 || v.Size == 2 || v.Size == 4 || v.Size == 8 {
			return v.Size
		}
		return 1
	case *btf.Array:
		return btfAlignment(v.Type)
	case *btf.Struct:
		var maxAlign uint32 = 1
		for _, m := range v.Members {
			if a := btfAlignment(m.Type); a > maxAlign {
				maxAlign = a
			}
		}
		return maxAlign
	case *btf.Union:
		var maxAlign uint32 = 1
		for _, m := range v.Members {
			if a := btfAlignment(m.Type); a > maxAlign {
				maxAlign = a
			}
		}
		return maxAlign
	}
	return 1
}

// unionBackingStorage picks the smallest aligned-array Go type for a
// union's backing store. Falls back to [N]byte (alignment 1) when
// the size doesn't divide cleanly by the desired alignment.
func unionBackingStorage(size, align uint32) string {
	switch {
	case align >= 8 && size%8 == 0:
		return fmt.Sprintf("_data [%d]uint64", size/8)
	case align >= 4 && size%4 == 0:
		return fmt.Sprintf("_data [%d]uint32", size/4)
	case align >= 2 && size%2 == 0:
		return fmt.Sprintf("_data [%d]uint16", size/2)
	}
	return fmt.Sprintf("_data [%d]byte", size)
}

// classifyKind maps a rendered Go type back to an IR Kind. Used so
// Phase 4 can decide what to downgrade. This is a coarse classifier;
// fine-grained discrimination is not needed by Phase 4.
func classifyKind(goType string) types.Kind {
	switch {
	case strings.HasPrefix(goType, "["):
		return types.KindArray
	case strings.HasPrefix(goType, "Pointer["):
		return types.KindPointer
	case goType == "uint8" || goType == "uint16" || goType == "uint32" || goType == "uint64",
		goType == "int8" || goType == "int16" || goType == "int32" || goType == "int64",
		goType == "float32" || goType == "float64",
		goType == "uintptr",
		goType == "bool":
		return types.KindPrimitive
	}
	return types.KindNamedStruct // best-effort; unions also land here
}
