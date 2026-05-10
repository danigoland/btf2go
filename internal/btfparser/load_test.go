package btfparser

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from the test source file until it finds a directory
// containing go.mod, which is the module root. Works for both the main
// worktree and git sub-worktrees.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	dir := filepath.Dir(thisFile)
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repo root (no go.mod found walking up from test file)")
		}
		dir = parent
	}
}

// TestLoadNoBTFSection verifies that Load returns a descriptive, actionable
// error message when the ELF has no .BTF section.
//
// This is the symptom produced when bpf-linker's LLVM version mismatches the
// system LLVM: the build "succeeds" but emits an ELF with no BTF debug info.
// Previously btf2go inspect printed only "btf: not found", giving users zero
// signal about the linker mismatch.
//
// The error message must contain all three of:
//   - "no BTF section" — names what is missing
//   - "bpf-linker" or "LLVM version" — points at the common cause
//   - "readelf -S" — tells the user how to verify
func TestLoadNoBTFSection(t *testing.T) {
	elfPath := filepath.Join(repoRoot(t), "tests", "fixtures", "nobtf", "nobtf.elf")

	_, err := Load(elfPath)
	if err == nil {
		t.Fatal("Load returned nil error for a BTF-less ELF; expected a descriptive error")
	}

	msg := err.Error()

	for _, substr := range []string{"no BTF section", "readelf -S"} {
		if !strings.Contains(msg, substr) {
			t.Errorf("error message missing %q\nfull message:\n  %s", substr, msg)
		}
	}

	if !strings.Contains(msg, "bpf-linker") && !strings.Contains(msg, "LLVM version") {
		t.Errorf("error message should mention 'bpf-linker' or 'LLVM version' as the common cause\nfull message:\n  %s", msg)
	}
}
