// Package verify is the layout assertion for the C fixture's golden
// output. It imports the committed eventspkg directly and uses
// unsafe.Offsetof / Sizeof to confirm that the Go compiler lays out
// the generated struct exactly the way the BTF graph said it should.
//
// This is the test that catches alignment-pass bugs end-to-end: if
// the alignment calculator inserted the wrong padding, or the packed-
// downgrade missed a misaligned primitive, the generated struct's
// Go-side offsets would not match the JSON sidecar and this test
// would fail.
package verify

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"unsafe"

	"github.com/danigoland/btf2go/tests/fixtures/c/eventspkg"
)

type structLayout struct {
	Size   uint32            `json:"size"`
	Fields map[string]uint32 `json:"fields"`
}

func loadLayout(t *testing.T) map[string]structLayout {
	t.Helper()
	bs, err := os.ReadFile(filepath.Join("..", "events.layout.json"))
	if err != nil {
		t.Fatal(err)
	}
	var doc map[string]structLayout
	if err := json.Unmarshal(bs, &doc); err != nil {
		t.Fatal(err)
	}
	return doc
}

// TestEventsLayoutSize asserts that the Go struct's Sizeof matches
// the BTF-declared size from the layout sidecar.
func TestEventsLayoutSize(t *testing.T) {
	doc := loadLayout(t)
	want := doc["EventsT"]
	var v eventspkg.EventsT
	if got := uint32(unsafe.Sizeof(v)); got != want.Size {
		t.Fatalf("EventsT size: got %d, want %d", got, want.Size)
	}
}

// TestEventsLayoutOffsets asserts every named field in the layout
// sidecar matches the Go struct's actual byte offset.
func TestEventsLayoutOffsets(t *testing.T) {
	doc := loadLayout(t)
	want := doc["EventsT"]
	var v eventspkg.EventsT
	rt := reflect.TypeOf(v)
	for name, wantOffset := range want.Fields {
		f, ok := rt.FieldByName(name)
		if !ok {
			t.Errorf("field %s not found on EventsT", name)
			continue
		}
		if got := uint32(f.Offset); got != wantOffset {
			t.Errorf("field %s offset: got %d, want %d", name, got, wantOffset)
		}
	}
}

// TestBitfieldRoundTrip exercises the generated Get/Set accessors for
// every bitfield in EventsT. It also verifies that setting one
// bitfield does not corrupt the others — the most common bug class
// for hand-rolled bit-shift-and-mask code.
func TestBitfieldRoundTrip(t *testing.T) {
	var v eventspkg.EventsT
	v.SetFlagA(1)
	v.SetFlagB(0)
	v.SetPrio(42)

	if got := v.GetFlagA(); got != 1 {
		t.Errorf("FlagA after Set(1): got %d, want 1", got)
	}
	if got := v.GetFlagB(); got != 0 {
		t.Errorf("FlagB after Set(0): got %d, want 0", got)
	}
	if got := v.GetPrio(); got != 42 {
		t.Errorf("Prio after Set(42): got %d, want 42", got)
	}

	// Cross-field non-corruption: changing FlagA must leave FlagB and
	// Prio intact. This is what catches off-by-one mask errors that
	// only manifest when bits in the same byte are written.
	v.SetFlagA(0)
	if got := v.GetFlagB(); got != 0 {
		t.Errorf("FlagB corrupted by SetFlagA(0): got %d, want 0", got)
	}
	if got := v.GetPrio(); got != 42 {
		t.Errorf("Prio corrupted by SetFlagA(0): got %d, want 42", got)
	}

	// Boundary: prio is 6 bits — set max, set min.
	v.SetPrio(63)
	if got := v.GetPrio(); got != 63 {
		t.Errorf("Prio max: got %d, want 63", got)
	}
	v.SetPrio(0)
	if got := v.GetPrio(); got != 0 {
		t.Errorf("Prio zero: got %d, want 0", got)
	}
	// And FlagA/B still intact.
	if got := v.GetFlagA(); got != 0 {
		t.Errorf("FlagA corrupted across Prio sweep: got %d", got)
	}
}

// TestUnionAccessorsRoundTrip exercises the union As<Member>/SetAs
// path. The union holds 8 bytes of storage; PayloadT.Raw is u64 and
// PayloadT.Pair is {u32 lo, u32 hi}. Writing through one accessor
// must be visible through the other (same underlying memory).
func TestUnionAccessorsRoundTrip(t *testing.T) {
	var p eventspkg.PayloadT
	p.SetAsRaw(0x1234567890ABCDEF)

	if got := *p.AsRaw(); got != 0x1234567890ABCDEF {
		t.Errorf("AsRaw round-trip: got 0x%x, want 0x1234567890ABCDEF", got)
	}

	// Same memory viewed as inner_t {lo, hi} on a little-endian host.
	pair := *p.AsPair()
	if pair.Lo != 0x90ABCDEF || pair.Hi != 0x12345678 {
		t.Errorf("AsPair view: got {Lo: 0x%x, Hi: 0x%x}, want {0x90ABCDEF, 0x12345678}",
			pair.Lo, pair.Hi)
	}
}
