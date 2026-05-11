package btfparser

import (
	"reflect"
	"testing"
)

func TestDecodeMangled(t *testing.T) {
	tests := []struct {
		name     string
		in       string
		wantOK   bool
		wantHead string
		wantArgs []string
	}{
		{
			name:   "plain identifier",
			in:     "ScaffoldPing",
			wantOK: false,
		},
		{
			name:     "simple two-arg",
			in:       "HashMap_3C_u64_2C__20_Foo_3E_",
			wantOK:   true,
			wantHead: "HashMap",
			wantArgs: []string{"u64", "Foo"},
		},
		{
			name:     "single arg",
			in:       "Array_3C_Foo_3E_",
			wantOK:   true,
			wantHead: "Array",
			wantArgs: []string{"Foo"},
		},
		{
			name:     "nested generic",
			in:       "HashMap_3C_u64_2C__20_Vec_3C_u8_3E__3E_",
			wantOK:   true,
			wantHead: "HashMap",
			wantArgs: []string{"u64", "Vec<u8>"},
		},
		{
			name:     "module path inner",
			in:       "HashMap_3C_u64_2C__20_firelxc_common_3A__3A_ScaffoldPing_3E_",
			wantOK:   true,
			wantHead: "HashMap",
			wantArgs: []string{"u64", "firelxc_common::ScaffoldPing"},
		},
		{
			name:   "unmatched open",
			in:     "HashMap_3C_u64",
			wantOK: false,
		},
		{
			name:   "unmatched close",
			in:     "u64_3E_",
			wantOK: false,
		},
		{
			name:   "empty",
			in:     "",
			wantOK: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := DecodeMangled(tc.in)
			if ok != tc.wantOK {
				t.Fatalf("ok = %v, want %v (got=%+v)", ok, tc.wantOK, got)
			}
			if !ok {
				return
			}
			if got.Head != tc.wantHead {
				t.Errorf("Head = %q, want %q", got.Head, tc.wantHead)
			}
			if !reflect.DeepEqual(got.Args, tc.wantArgs) {
				t.Errorf("Args = %v, want %v", got.Args, tc.wantArgs)
			}
		})
	}
}
