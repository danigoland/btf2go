package btfparser

import (
	"errors"
	"testing"

	"github.com/cilium/ebpf/btf"
)

// makeSpecWithWrapper builds a spec containing a struct named wrapperName
// plus one struct per extras entry.
func makeSpecWithWrapper(t *testing.T, wrapperName string, extras ...string) *btf.Spec {
	t.Helper()
	names := append([]string{wrapperName}, extras...)
	return makeSpec(t, names...)
}

// containsType reports whether any element of types has the given name.
func containsType(types []btf.Type, name string) bool {
	for _, t := range types {
		if t.TypeName() == name {
			return true
		}
	}
	return false
}

// typeNames returns the TypeName() of each element.
func typeNames(types []btf.Type) []string {
	out := make([]string, len(types))
	for i, t := range types {
		out[i] = t.TypeName()
	}
	return out
}

// intSliceEq reports whether a and b are equal.
func intSliceEq(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestBridgeAya_NilSpec(t *testing.T) {
	_, err := BridgeAya(nil, BridgeOptions{})
	if err == nil {
		t.Fatalf("expected error for nil spec, got nil")
	}
}

func TestBridgeAya_DefaultHashMap(t *testing.T) {
	// HashMap_3C_u64_2C__20_Foo_3E_ decodes to HashMap<u64, Foo>
	spec := makeSpecWithWrapper(t, "HashMap_3C_u64_2C__20_Foo_3E_", "Foo")
	got, err := BridgeAya(spec, BridgeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsType(got, "Foo") {
		t.Errorf("expected Foo in output, got %v", typeNames(got))
	}
}

func TestBridgeAya_DefaultArray(t *testing.T) {
	// Array_3C_Bar_3E_ decodes to Array<Bar>
	spec := makeSpecWithWrapper(t, "Array_3C_Bar_3E_", "Bar")
	got, err := BridgeAya(spec, BridgeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsType(got, "Bar") {
		t.Errorf("expected Bar in output, got %v", typeNames(got))
	}
}

func TestBridgeAya_MissingInnerType(t *testing.T) {
	// HashMap<u64, Missing> — Missing is not in the spec
	spec := makeSpecWithWrapper(t, "HashMap_3C_u64_2C__20_Missing_3E_")
	_, err := BridgeAya(spec, BridgeOptions{})
	if err == nil {
		t.Fatal("expected error for missing inner type, got nil")
	}
	var nfe *LookupNotFoundError
	if !errors.As(err, &nfe) {
		t.Fatalf("expected *LookupNotFoundError wrapped in error, got %T: %v", err, err)
	}
	msg := err.Error()
	if !containsStr(msg, "Missing") {
		t.Errorf("error message missing %q: %q", "Missing", msg)
	}
	if !containsStr(msg, "HashMap") {
		t.Errorf("error message missing %q: %q", "HashMap", msg)
	}
}

func TestBridgeAya_CustomBridge(t *testing.T) {
	// MyMap<u32, Payload> — custom bridge entry
	spec := makeSpecWithWrapper(t, "MyMap_3C_u32_2C__20_Payload_3E_", "Payload")
	opts := BridgeOptions{
		Extra: map[string]BridgeSpec{
			"MyMap": {Arity: 2, LayoutBearing: []int{1}},
		},
	}
	got, err := BridgeAya(spec, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !containsType(got, "Payload") {
		t.Errorf("expected Payload in output, got %v", typeNames(got))
	}
}

func TestBridgeAya_IgnoresUnknownWrapper(t *testing.T) {
	// UnknownWrapper<u32, Inner> — not in bridge table, Inner should NOT appear
	spec := makeSpecWithWrapper(t, "UnknownWrapper_3C_u32_2C__20_Inner_3E_", "Inner")
	got, err := BridgeAya(spec, BridgeOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if containsType(got, "Inner") {
		t.Errorf("expected Inner to be absent from output, got %v", typeNames(got))
	}
}

func TestParseBridgeOverride(t *testing.T) {
	tests := []struct {
		input   string
		name    string
		spec    BridgeSpec
		wantErr bool
	}{
		{
			input: "MyMap=2:1",
			name:  "MyMap",
			spec:  BridgeSpec{Arity: 2, LayoutBearing: []int{1}},
		},
		{
			input: "RingBuf=1:0",
			name:  "RingBuf",
			spec:  BridgeSpec{Arity: 1, LayoutBearing: []int{0}},
		},
		{
			input: "Both=2:0,1",
			name:  "Both",
			spec:  BridgeSpec{Arity: 2, LayoutBearing: []int{0, 1}},
		},
		{input: "NoArity=:0", wantErr: true},
		{input: "MissingPositions=2:", wantErr: true},
		{input: "OutOfRange=2:5", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.input, func(t *testing.T) {
			name, spec, err := ParseBridgeOverride(tc.input)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %q, got nil", tc.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if name != tc.name {
				t.Errorf("name: got %q, want %q", name, tc.name)
			}
			if spec.Arity != tc.spec.Arity {
				t.Errorf("arity: got %d, want %d", spec.Arity, tc.spec.Arity)
			}
			if !intSliceEq(spec.LayoutBearing, tc.spec.LayoutBearing) {
				t.Errorf("LayoutBearing: got %v, want %v", spec.LayoutBearing, tc.spec.LayoutBearing)
			}
		})
	}
}

// containsStr is a local helper to avoid importing strings in test file.
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || findStr(s, sub))
}

func findStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
