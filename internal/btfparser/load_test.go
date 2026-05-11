package btfparser

import (
	"encoding/binary"
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
// error message when the ELF has no .BTF section, using the committed fixture.
func TestLoadNoBTFSection(t *testing.T) {
	elfPath := filepath.Join(repoRoot(t), "tests", "fixtures", "nobtf", "nobtf.elf")

	_, err := Load(elfPath)
	if err == nil {
		t.Fatal("Load returned nil error for a BTF-less ELF; expected a descriptive error")
	}

	msg := err.Error()

	for _, substr := range []string{"no .BTF section", "readelf -S"} {
		if !strings.Contains(msg, substr) {
			t.Errorf("error message missing %q\nfull message:\n  %s", substr, msg)
		}
	}

	if !strings.Contains(msg, "bpf-linker") && !strings.Contains(msg, "rustflags") {
		t.Errorf("error message should mention 'bpf-linker' or 'rustflags'\nfull message:\n  %s", msg)
	}
}

// TestLoad_NoBTF_DiagnosticText verifies all required strings in the v0.4
// BTF-less diagnostic using a minimal in-memory ELF without a .BTF section.
func TestLoad_NoBTF_DiagnosticText(t *testing.T) {
	// Write a minimal ELF without a .BTF section to a temp path.
	elfPath := writeBTFlessELF(t)

	_, err := Load(elfPath)
	if err == nil {
		t.Fatalf("expected error for BTF-less ELF")
	}
	msg := err.Error()
	for _, want := range []string{
		"no .BTF section",
		"link-arg=--btf",
		"rustflags",
		"bpf-linker",
		"For clang users",
		"zig-toolchain",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("diagnostic missing %q\nfull message:\n  %s", want, msg)
		}
	}
}

// writeBTFlessELF writes a minimal valid ELF64 with no .BTF section
// to a temp file and returns its path.
func writeBTFlessELF(t *testing.T) string {
	t.Helper()
	hdr := make([]byte, 64+64)
	copy(hdr[0:4], []byte{0x7f, 'E', 'L', 'F'})
	hdr[4] = 2                                     // 64-bit
	hdr[5] = 1                                     // little-endian
	hdr[6] = 1                                     // EI_VERSION
	binary.LittleEndian.PutUint16(hdr[16:18], 1)   // e_type=ET_REL
	binary.LittleEndian.PutUint16(hdr[18:20], 247) // e_machine=EM_BPF
	binary.LittleEndian.PutUint32(hdr[20:24], 1)   // e_version
	binary.LittleEndian.PutUint64(hdr[40:48], 64)  // e_shoff
	binary.LittleEndian.PutUint16(hdr[52:54], 64)  // e_ehsize
	binary.LittleEndian.PutUint16(hdr[58:60], 64)  // e_shentsize
	binary.LittleEndian.PutUint16(hdr[60:62], 1)   // e_shnum
	binary.LittleEndian.PutUint16(hdr[62:64], 0)   // e_shstrndx

	path := filepath.Join(t.TempDir(), "no-btf.elf")
	if err := os.WriteFile(path, hdr, 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}
