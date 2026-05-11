// Package tests — end-to-end integration tests for the rust-aya fixtures.
//
// Each test shells out to `go run ./cmd/btf2go generate …` so it exercises
// the same code path real users hit.  Run from the repo root with:
//
//	go test ./tests/ -run TestRustAya -v
//
// Regenerate goldens after an intentional output change:
//
//	UPDATE_GOLDEN=1 go test ./tests/ -run TestRustAya -v
package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// ─────────────────────────────────────────────────────────────────────────────
// TestRustAya_MapsHappy_NoAya
//
// Runs `btf2go generate --pkg fixture --type BareStruct` (NO --aya) against
// maps-happy.elf.  Expected output matches
// fixtures/rust-aya/maps-happy/golden_no_aya/types.go.
// ─────────────────────────────────────────────────────────────────────────────
func TestRustAya_MapsHappy_NoAya(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "tests/fixtures/rust-aya/maps-happy")
	got := runAyaGenerate(t, root, filepath.Join(dir, "maps-happy.elf"), []string{
		"--pkg", "fixture",
		"--type", "BareStruct",
		"--source-name", "maps-happy.elf",
	})
	goldenPath := filepath.Join(dir, "golden_no_aya/types.go")
	checkOrUpdateAyaGolden(t, goldenPath, got)
}

// ─────────────────────────────────────────────────────────────────────────────
// TestRustAya_MapsHappy_Aya
//
// Runs `btf2go generate --pkg fixture --aya` against maps-happy.elf.
// Expected output matches fixtures/rust-aya/maps-happy/golden_aya/types.go.
// ─────────────────────────────────────────────────────────────────────────────
func TestRustAya_MapsHappy_Aya(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "tests/fixtures/rust-aya/maps-happy")
	got := runAyaGenerate(t, root, filepath.Join(dir, "maps-happy.elf"), []string{
		"--pkg", "fixture",
		"--aya",
		"--source-name", "maps-happy.elf",
	})
	goldenPath := filepath.Join(dir, "golden_aya/types.go")
	checkOrUpdateAyaGolden(t, goldenPath, got)
}

// ─────────────────────────────────────────────────────────────────────────────
// TestRustAya_MapsMissingExport_Errors
//
// Runs `btf2go generate --aya` against an ELF whose V-type is not in BTF.
// Expects nonzero exit + stderr containing the diagnostic substrings
// captured in expected.err.
// ─────────────────────────────────────────────────────────────────────────────
func TestRustAya_MapsMissingExport_Errors(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "tests/fixtures/rust-aya/maps-missing-export")
	elfPath := filepath.Join(dir, "maps-missing-export.elf")
	tmpOut := filepath.Join(t.TempDir(), "out.go")

	cmd := exec.Command("go", "run", "./cmd/btf2go",
		"generate", "--elf", elfPath, "--pkg", "fixture", "--aya", "--out", tmpOut)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err == nil {
		t.Fatalf("expected nonzero exit from btf2go generate, got success")
	}
	got := stderr.String()

	// Don't full-string-compare — the cobra usage block contains flag
	// descriptions that may change as we add flags.  Instead assert the
	// diagnostic's load-bearing substrings from expected.err.
	for _, want := range []string{
		"aya bridge",
		"ScaffoldPing",
		"HashMap",
		"not resolvable",
		"not found",
		"tried:",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("stderr missing %q\n--- got stderr ---\n%s", want, got)
		}
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// TestRustAya_MultiElf_Shared
//
// Two-step --shared-out workflow: lsm.elf then xdp.elf, both routing
// BinaryIdentity to a shared file.  Verifies the three goldens match.
//
// The shared file is deterministic (no per-run timestamps since Gap 8 fix),
// so we compare directly without normalization.
// ─────────────────────────────────────────────────────────────────────────────
func TestRustAya_MultiElf_Shared(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(root, "tests/fixtures/rust-aya/multi-elf-shared")
	tmp := t.TempDir()
	sharedOut := filepath.Join(tmp, "shared.go")
	lsmOut := filepath.Join(tmp, "lsm_types.go")
	xdpOut := filepath.Join(tmp, "xdp_types.go")

	runAyaGenerateInto(t, root, filepath.Join(dir, "lsm.elf"), []string{
		"--pkg", "fixture", "--aya",
		"--shared-out", sharedOut,
		"--shared-type", "BinaryIdentity",
		"--source-name", "lsm.elf",
	}, lsmOut)

	runAyaGenerateInto(t, root, filepath.Join(dir, "xdp.elf"), []string{
		"--pkg", "fixture", "--aya",
		"--shared-out", sharedOut,
		"--shared-type", "BinaryIdentity",
		"--source-name", "xdp.elf",
	}, xdpOut)

	sharedGot, err := os.ReadFile(sharedOut)
	if err != nil {
		t.Fatalf("read shared output: %v", err)
	}
	lsmGot, err := os.ReadFile(lsmOut)
	if err != nil {
		t.Fatalf("read lsm output: %v", err)
	}
	xdpGot, err := os.ReadFile(xdpOut)
	if err != nil {
		t.Fatalf("read xdp output: %v", err)
	}

	// Shared file is now deterministic (no per-run timestamps) — compare directly.
	checkOrUpdateAyaGolden(t, filepath.Join(dir, "golden_shared/types.go"), sharedGot)
	checkOrUpdateAyaGolden(t, filepath.Join(dir, "golden_lsm/types.go"), lsmGot)
	checkOrUpdateAyaGolden(t, filepath.Join(dir, "golden_xdp/types.go"), xdpGot)
}

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

// repoRoot returns the absolute path to the repository root (one level above
// the tests/ package directory).
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("repo root: %v", err)
	}
	return root
}

// runAyaGenerate runs btf2go generate and returns the contents of the output
// file on success.
func runAyaGenerate(t *testing.T, root, elfPath string, extraArgs []string) []byte {
	t.Helper()
	out := filepath.Join(t.TempDir(), "out.go")
	runAyaGenerateInto(t, root, elfPath, extraArgs, out)
	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read generate output: %v", err)
	}
	return data
}

// runAyaGenerateInto runs btf2go generate writing to outPath.
func runAyaGenerateInto(t *testing.T, root, elfPath string, extraArgs []string, outPath string) {
	t.Helper()
	args := []string{"run", "./cmd/btf2go", "generate",
		"--elf", elfPath,
		"--out", outPath,
	}
	args = append(args, extraArgs...)
	cmd := exec.Command("go", args...)
	cmd.Dir = root
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		t.Fatalf("btf2go generate failed: %v\nstderr:\n%s", err, stderr.String())
	}
}

// checkOrUpdateAyaGolden compares got against the golden at path.
// With UPDATE_GOLDEN=1 it overwrites the golden instead.
func checkOrUpdateAyaGolden(t *testing.T, path string, got []byte) {
	t.Helper()
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("write golden %s: %v", path, err)
		}
		t.Logf("golden updated: %s", path)
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read golden %s: %v\n(run with UPDATE_GOLDEN=1 to create it)", path, err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("golden mismatch: %s\n--- diff first divergence ---", path)
		showAyaFirstDiff(t, got, want)
		t.Logf("run with UPDATE_GOLDEN=1 to update the golden")
	}
}

// showAyaFirstDiff logs the first diverging line between got and want.
func showAyaFirstDiff(t *testing.T, got, want []byte) {
	t.Helper()
	gotLines := strings.Split(string(got), "\n")
	wantLines := strings.Split(string(want), "\n")
	n := len(gotLines)
	if len(wantLines) < n {
		n = len(wantLines)
	}
	for i := 0; i < n; i++ {
		if gotLines[i] != wantLines[i] {
			t.Logf("first diff at line %d:\n  got:  %q\n  want: %q", i+1, gotLines[i], wantLines[i])
			return
		}
	}
	if len(gotLines) != len(wantLines) {
		t.Logf("length differs: got %d lines, want %d", len(gotLines), len(wantLines))
	}
}
