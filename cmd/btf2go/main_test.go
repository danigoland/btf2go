package main

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// buildBinary compiles the btf2go binary into a temp dir and returns its path.
// Uses the module root so that the build picks up all dependencies.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "btf2go")
	cmd := exec.Command("go", "build", "-o", bin, "./cmd/btf2go")
	cmd.Dir = moduleRoot(t)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

// moduleRoot walks up from the test file's directory looking for go.mod.
func moduleRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find module root (no go.mod found)")
		}
		dir = parent
	}
}

// TestVersionSubcommand asserts that `btf2go version` prints a version string
// starting with "v" to stdout and exits 0.
func TestVersionSubcommand(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("btf2go version exited non-zero: %v\nstdout: %s\nstderr: %s",
			err, stdout.String(), stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	if !strings.HasPrefix(got, "v") {
		t.Errorf("expected version string starting with 'v', got: %q", got)
	}
}

// TestVersionFlag asserts that `btf2go --version` prints a version line
// containing "btf2go version v" to stdout and exits 0.
func TestVersionFlag(t *testing.T) {
	bin := buildBinary(t)

	cmd := exec.Command(bin, "--version")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("btf2go --version exited non-zero: %v\nstdout: %s\nstderr: %s",
			err, stdout.String(), stderr.String())
	}

	got := strings.TrimSpace(stdout.String())
	// cobra's default version template prints "btf2go version <ver>"
	if !strings.Contains(got, "btf2go version v") {
		t.Errorf("expected version line containing 'btf2go version v', got: %q", got)
	}
}

// TestGenerateSuccessLine asserts that a successful `btf2go generate` run
// prints "Generated: <path>" to stderr.
func TestGenerateSuccessLine(t *testing.T) {
	bin := buildBinary(t)

	// Find a fixture ELF to use.
	root := moduleRoot(t)
	elf := filepath.Join(root, "tests", "fixtures", "c", "events.elf")
	if _, err := os.Stat(elf); err != nil {
		t.Skipf("fixture ELF not found at %s: %v", elf, err)
	}

	outFile := filepath.Join(t.TempDir(), "out.go")

	cmd := exec.Command(bin, "generate",
		"--elf", elf,
		"--pkg", "testpkg",
		"--out", outFile,
	)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		t.Fatalf("btf2go generate failed: %v\nstdout: %s\nstderr: %s",
			err, stdout.String(), stderr.String())
	}

	stderrStr := stderr.String()
	if !strings.Contains(stderrStr, "Generated: ") {
		t.Errorf("expected 'Generated: ' in stderr, got: %q", stderrStr)
	}
	if !strings.Contains(stderrStr, outFile) {
		t.Errorf("expected output path %q in stderr line, got: %q", outFile, stderrStr)
	}
}
