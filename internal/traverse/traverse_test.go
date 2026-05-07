package traverse

import (
	"testing"

	"github.com/cilium/ebpf/btf"
)

func TestGoIntType(t *testing.T) {
	cases := []struct {
		size   uint32
		signed bool
		want   string
	}{
		{1, false, "uint8"}, {1, true, "int8"},
		{2, false, "uint16"}, {2, true, "int16"},
		{4, false, "uint32"}, {4, true, "int32"},
		{8, false, "uint64"}, {8, true, "int64"},
		{3, false, "[3]byte"},
	}
	for _, c := range cases {
		var enc btf.IntEncoding
		if c.signed {
			enc = btf.Signed
		}
		i := &btf.Int{Size: c.size, Encoding: enc}
		if got := goIntType(i); got != c.want {
			t.Errorf("size=%d signed=%v: got %q want %q", c.size, c.signed, got, c.want)
		}
	}
}

func TestGoFloatType(t *testing.T) {
	cases := []struct {
		size uint32
		want string
	}{
		{4, "float32"},
		{8, "float64"},
		{2, "[2]byte"},
		{16, "[16]byte"}, // long double on some toolchains
	}
	for _, c := range cases {
		f := &btf.Float{Size: c.size}
		if got := goFloatType(f); got != c.want {
			t.Errorf("size=%d: got %q, want %q", c.size, got, c.want)
		}
	}
}
