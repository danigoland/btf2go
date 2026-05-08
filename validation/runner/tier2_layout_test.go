package main

import "testing"

func TestBtfLayoutsOnCFixture(t *testing.T) {
	got, err := btfLayouts("../../tests/fixtures/c/events.elf")
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := got["events_t"]
	if !ok {
		t.Fatalf("events_t missing; have %v", keys(got))
	}
	if ev.Size != 48 {
		t.Errorf("events_t size: got %d, want 48", ev.Size)
	}
	if ev.Fields["pid"] != 4 {
		t.Errorf("events_t.pid offset: got %d, want 4", ev.Fields["pid"])
	}
	if ev.Fields["ts"] != 16 {
		t.Errorf("events_t.ts offset: got %d, want 16", ev.Fields["ts"])
	}
}

func keys(m map[string]goLayout) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
