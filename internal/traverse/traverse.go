// Package traverse implements Phase 3: convert a slice of btf.Type
// (produced by btfparser.Resolve) into a types.GoFile IR (without
// alignment — that is Phase 4's job).
package traverse

import (
	"fmt"

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
	case *btf.Void:
		return "uintptr", nil
	case *btf.Enum:
		return b.declareEnum(v)
	case *btf.Array:
		return b.declareArray(v, parentField)
	case *btf.Pointer:
		return b.declarePointer(v, parentField)
	case *btf.Typedef:
		return b.declare(v.Type, parentField)
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

// goIntType maps btf.Int to a Go primitive. Width must be 1, 2, 4, or 8.
func goIntType(i *btf.Int) string {
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
	name := btfparser.SanitizeName(e.Name)
	if name == "_anon" {
		b.anonN++
		name = btfparser.AnonName("", "", b.anonN-1)
	}
	if _, exists := b.named[e]; exists {
		return name, nil
	}
	underlying := "uint32"
	if e.Size == 8 {
		underlying = "uint64"
	}
	g := types.GoEnum{Name: name, Underlying: underlying}
	for _, val := range e.Values {
		g.Values = append(g.Values, types.GoEnumValue{
			Name:  name + "_" + btfparser.SanitizeName(val.Name),
			Value: int64(val.Value),
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

// declareStruct and declareUnion are implemented in Task 11.
func (b *builder) declareStruct(s *btf.Struct, parentField string) (string, error) {
	return "", fmt.Errorf("struct traversal not yet implemented")
}
func (b *builder) declareUnion(u *btf.Union, parentField string) (string, error) {
	return "", fmt.Errorf("union traversal not yet implemented")
}
