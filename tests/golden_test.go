// Package tests holds end-to-end tests for btf2go. Run from the repo
// root with `go test ./tests/...`. The golden test runs btf2go in-
// process against committed .elf fixtures and compares the output
// against committed .golden.go files.
//
// To regenerate goldens after an intentional output change, run:
//
//	UPDATE_GOLDEN=1 go test ./tests/...
package tests

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGoldenC compares btf2go output against the committed golden for
// the C fixture. Run with UPDATE_GOLDEN=1 to overwrite the committed
// golden after an intentional output change.
func TestGoldenC(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	// Use the path relative to the repo root so the "Source:" line in
	// the generated header is portable across machines (the golden
	// records "tests/fixtures/c/events.elf", not an absolute path).
	const elfRel = "tests/fixtures/c/events.elf"
	golden := filepath.Join(root, "tests", "fixtures", "c", "eventspkg", "events.go")

	tmp, err := os.CreateTemp("", "btf2go-*.go")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp.Name())
	tmp.Close()

	cmd := exec.Command("go", "run", "./cmd/btf2go", "generate",
		"--elf", elfRel,
		"--pkg", "eventspkg",
		"--out", tmp.Name(),
		"--type", "events_t",
		"--no-map-types",
	)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("btf2go generate: %v\n%s", err, out)
	}

	got, err := os.ReadFile(tmp.Name())
	if err != nil {
		t.Fatal(err)
	}
	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Log("golden updated at", golden)
		return
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatalf("read golden: %v", err)
	}
	if !bytes.Equal(got, want) {
		t.Fatalf("golden mismatch.\n--- want ---\n%s\n--- got ---\n%s", want, got)
	}
}
