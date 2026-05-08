package main

import (
	"os"
	"testing"
)

// TestRunTier2OneELF_NoBTF verifies that an ELF compiled with -fno-BTF
// is reported as SKIP (not FAIL) with a clear reason.
func TestRunTier2OneELF_NoBTF(t *testing.T) {
	const elfPath = "../../validation/corpus/c/cilium-ebpf-testdata/testdata/loader_nobtf-el.elf"
	if _, err := os.Stat(elfPath); err != nil {
		t.Skip("corpus not materialised — run validation/refresh.sh first")
	}

	result := runTier2OneELF(elfPath, "testpkg", "btf2go")
	if result.Status != StatusSkip {
		t.Errorf("expected StatusSkip for no-BTF ELF, got %v (detail: %s)", result.Status, result.Detail)
	}
	if result.SkipReason == "" {
		t.Error("expected non-empty SkipReason for no-BTF ELF")
	}
	t.Logf("SkipReason: %s", result.SkipReason)
}

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
