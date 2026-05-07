package align

import (
	"fmt"

	"github.com/danigoland/btf2go/internal/types"
)

// Apply mutates s in place: inserts synthetic _padN fields wherever the
// declared offset of a field exceeds the running cursor, and a trailing
// _padN if the summed field bytes are less than s.Size.
//
// Later steps in this task series extend Apply with packed-primitive
// downgrade and bitfield-block collapse.
func Apply(s *types.GoStruct) error {
	out := make([]types.GoField, 0, len(s.Fields)*2)
	var cursor uint32
	padN := 0
	for _, f := range s.Fields {
		if f.Offset < cursor {
			return fmt.Errorf("struct %s: field %s offset %d < cursor %d (overlap)", s.Name, f.Name, f.Offset, cursor)
		}
		if f.Offset > cursor {
			out = append(out, padField(padN, cursor, f.Offset-cursor))
			padN++
		}
		out = append(out, f)
		cursor = f.Offset + f.Size
	}
	if cursor < s.Size {
		out = append(out, padField(padN, cursor, s.Size-cursor))
	}
	s.Fields = out
	return nil
}

func padField(n int, offset, size uint32) types.GoField {
	return types.GoField{
		Name:   fmt.Sprintf("_pad%d", n),
		Kind:   types.KindRawBytes,
		GoType: fmt.Sprintf("[%d]byte", size),
		Offset: offset,
		Size:   size,
		IsPad:  true,
	}
}
