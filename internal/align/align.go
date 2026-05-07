package align

import (
	"fmt"

	"github.com/danigoland/btf2go/internal/types"
)

// Apply mutates s in place: inserts synthetic _padN fields wherever the
// declared offset of a field exceeds the running cursor, and a trailing
// _padN if the summed field bytes are less than s.Size. It also
// downgrades misaligned packed primitives to [N]byte and collapses
// contiguous bitfield runs into a single _bfN [storageBytes]byte
// storage field plus accessor metadata in s.Bitfields.
func Apply(s *types.GoStruct) error {
	out := make([]types.GoField, 0, len(s.Fields)*2)
	var cursor uint32
	padN, bfN := 0, 0
	i := 0
	for i < len(s.Fields) {
		f := s.Fields[i]
		if f.BitfieldBits > 0 {
			// Collect the contiguous bitfield run starting at i.
			runStart := i
			runByteOffset := f.Offset
			var maxBitEnd uint32
			for i < len(s.Fields) && s.Fields[i].BitfieldBits > 0 {
				m := s.Fields[i]
				bitEnd := (m.Offset-runByteOffset)*8 + m.BitOffset%8 + m.BitfieldBits
				// Note: BTF Member.Offset (which we copied to BitOffset) is the
				// absolute bit offset within the parent struct. Phase 3 must
				// set Offset = (Member.Offset/8) for the byte position and
				// BitOffset = Member.Offset (full bits) so we can recompute
				// the in-block bit offset here.
				if bitEnd > maxBitEnd {
					maxBitEnd = bitEnd
				}
				i++
			}
			storageBits := maxBitEnd
			storageBytes := (storageBits + 7) / 8
			storageName := fmt.Sprintf("_bf%d", bfN)
			bfN++
			// Match the regular-field overlap guard.
			if runByteOffset < cursor {
				return fmt.Errorf("struct %s: bitfield run at byte %d < cursor %d (overlap)", s.Name, runByteOffset, cursor)
			}
			// Insert leading padding if needed.
			if runByteOffset > cursor {
				out = append(out, padField(padN, cursor, runByteOffset-cursor))
				padN++
			}
			// Emit the storage field.
			out = append(out, types.GoField{
				Name: storageName, Kind: types.KindRawBytes,
				GoType: fmt.Sprintf("[%d]byte", storageBytes),
				Offset: runByteOffset, Size: storageBytes,
			})
			// Build accessors.
			block := types.GoBitfieldBlock{
				StorageField: storageName,
				StorageSize:  storageBytes,
			}
			for j := runStart; j < i; j++ {
				m := s.Fields[j]
				bitOffsetInBlock := m.BitOffset - runByteOffset*8
				block.Accessors = append(block.Accessors, types.GoBitAccessor{
					Name:      snakeToPascal(m.Name),
					BitOffset: bitOffsetInBlock,
					BitWidth:  m.BitfieldBits,
					Signed:    isSignedGoInt(m.GoType),
					GoType:    chooseAccessorGoType(m.BitfieldBits, isSignedGoInt(m.GoType)),
				})
			}
			s.Bitfields = append(s.Bitfields, block)
			cursor = runByteOffset + storageBytes
			continue
		}
		// Regular field.
		if f.Offset < cursor {
			return fmt.Errorf("struct %s: field %s offset %d < cursor %d (overlap)", s.Name, f.Name, f.Offset, cursor)
		}
		if f.Offset > cursor {
			out = append(out, padField(padN, cursor, f.Offset-cursor))
			padN++
		}
		out = append(out, downgradeIfMisaligned(f))
		cursor = f.Offset + f.Size
		i++
	}
	if cursor > s.Size {
		return fmt.Errorf("struct %s: total field bytes %d exceed declared size %d", s.Name, cursor, s.Size)
	}
	if cursor < s.Size {
		out = append(out, padField(padN, cursor, s.Size-cursor))
	}
	s.Fields = out
	return nil
}

func downgradeIfMisaligned(f types.GoField) types.GoField {
	if f.Kind != types.KindPrimitive && f.Kind != types.KindPointer {
		return f
	}
	natural := GoAlign(f.GoType)
	if natural == 0 || f.Offset%natural == 0 {
		return f
	}
	f.Kind = types.KindRawBytes
	f.GoType = fmt.Sprintf("[%d]byte", f.Size)
	return f
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

// snakeToPascal converts an identifier like "flag_a" or "frame.lo" to
// "FlagA" / "FrameLo". Digits are appended unchanged and leave the
// "uppercase the next letter" flag intact, so "flag_2a" becomes
// "Flag2A". Existing uppercase letters are preserved.
func snakeToPascal(s string) string {
	var b []rune
	upper := true
	for _, r := range s {
		switch {
		case r == '_' || r == '.':
			upper = true
		case r >= '0' && r <= '9':
			b = append(b, r)
		case r >= 'a' && r <= 'z':
			if upper {
				r -= 'a' - 'A'
			}
			upper = false
			b = append(b, r)
		case r >= 'A' && r <= 'Z':
			upper = false
			b = append(b, r)
		default:
			b = append(b, r)
		}
	}
	return string(b)
}

// chooseAccessorGoType picks the smallest unsigned/signed Go integer
// type that holds bitWidth bits.
func chooseAccessorGoType(bitWidth uint32, signed bool) string {
	prefix := "uint"
	if signed {
		prefix = "int"
	}
	switch {
	case bitWidth <= 8:
		return prefix + "8"
	case bitWidth <= 16:
		return prefix + "16"
	case bitWidth <= 32:
		return prefix + "32"
	default:
		return prefix + "64"
	}
}

func isSignedGoInt(goType string) bool {
	switch goType {
	case "int", "int8", "int16", "int32", "int64":
		return true
	}
	return false
}
