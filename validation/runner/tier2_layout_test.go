package main

import (
	"os"
	"strings"
	"testing"
)

// TestRunTier2OneELF_NoBTF verifies that an ELF compiled with -fno-BTF
// is reported as SKIP (not FAIL) with a clear reason.
func TestRunTier2OneELF_NoBTF(t *testing.T) {
	const elfPath = "../../validation/corpus/c/cilium-ebpf-testdata/testdata/loader_nobtf-el.elf"
	if _, err := os.Stat(elfPath); err != nil {
		if os.IsNotExist(err) {
			t.Skip("corpus not materialised — run validation/refresh.sh first")
		}
		t.Fatalf("stat %s: %v", elfPath, err)
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

// TestBtfLayouts_EmptyStructsExcluded verifies that btfLayouts drops
// zero-member BTF structs (kfunc-style opaque kernel shadows) so they
// don't produce spurious "not in generated output" diffs against
// parseGoLayouts (which already drops empty Go structs).
func TestBtfLayouts_EmptyStructsExcluded(t *testing.T) {
	const elfPath = "../../validation/corpus/c/cilium-ebpf-testdata/testdata/kfunc-el.elf"
	if _, err := os.Stat(elfPath); err != nil {
		if os.IsNotExist(err) {
			t.Skip("corpus not materialised — run validation/refresh.sh first")
		}
		t.Fatalf("stat %s: %v", elfPath, err)
	}

	layouts, err := btfLayouts(elfPath)
	if err != nil {
		t.Fatalf("btfLayouts: %v", err)
	}

	// These five are zero-member kfunc shadows; they must NOT appear in
	// the result (they would trigger "not in generated output" diffs).
	emptyKfuncShadows := []string{"BpfCtOpts", "BpfCpumask", "NfConn", "SkBuff", "BpfSockTuple"}
	for _, name := range emptyKfuncShadows {
		// Also check raw BTF names (lower-snake) that SanitizeName maps to the above.
		rawNames := []string{name, "bpf_ct_opts", "bpf_cpumask", "nf_conn", "__sk_buff", "bpf_sock_tuple"}
		_ = rawNames
		if _, ok := layouts[name]; ok {
			t.Errorf("btfLayouts should exclude empty struct %q but it is present", name)
		}
	}

	// Verify that we also exclude by raw BTF names (before SanitizeName).
	rawEmptyNames := []string{"bpf_ct_opts", "bpf_cpumask", "nf_conn", "__sk_buff", "bpf_sock_tuple"}
	for _, raw := range rawEmptyNames {
		if _, ok := layouts[raw]; ok {
			t.Errorf("btfLayouts should exclude empty struct (raw BTF name) %q but it is present", raw)
		}
	}
}

// TestRunTier2OneELF_KfuncNoSpuriousFail verifies that kfunc-el.elf does
// NOT produce "not in generated output" findings for zero-member shadow
// structs like SkBuff, NfConn, etc.
func TestRunTier2OneELF_KfuncNoSpuriousFail(t *testing.T) {
	const elfPath = "../../validation/corpus/c/cilium-ebpf-testdata/testdata/kfunc-el.elf"
	if _, err := os.Stat(elfPath); err != nil {
		if os.IsNotExist(err) {
			t.Skip("corpus not materialised — run validation/refresh.sh first")
		}
		t.Fatalf("stat %s: %v", elfPath, err)
	}

	result := runTier2OneELF(elfPath, "testpkg", "btf2go")
	if result.Status == StatusFail {
		// Fail only if the reason is a spurious empty-struct diff.
		if strings.Contains(result.Detail, "not in generated output") {
			t.Errorf("spurious 'not in generated output' diff — empty-struct filter asymmetry not fixed\nDetail:\n%s", result.Detail)
		}
	}
}

func keys(m map[string]goLayout) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
