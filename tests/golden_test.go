// Package tests holds end-to-end tests for btf2go. Run from the repo
// root with `go test ./tests/...`. Each golden test runs btf2go in-
// process against a committed .elf fixture and compares the output
// against a committed .golden.go file (under tests/fixtures/<lang>/
// eventspkg/events.go).
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

// goldenCase parameterizes the table-driven golden test so each
// language fixture (C, Rust/Aya, future Zig) only needs the elf path,
// the target package name, and the type whitelist.
type goldenCase struct {
	name      string
	elfRel    string // path relative to repo root
	pkg       string
	types     []string
	goldenRel string // path relative to repo root
}

// TestGolden runs btf2go against every committed fixture and diffs
// the output against the corresponding committed golden. Run with
// UPDATE_GOLDEN=1 to overwrite the goldens after an intentional
// output change.
func TestGolden(t *testing.T) {
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}

	cases := []goldenCase{
		{
			name:      "c",
			elfRel:    "tests/fixtures/c/events.elf",
			pkg:       "eventspkg",
			types:     []string{"events_t"},
			goldenRel: "tests/fixtures/c/eventspkg/events.go",
		},
		{
			name:      "rust",
			elfRel:    "tests/fixtures/rust/fixture.elf",
			pkg:       "eventspkg",
			types:     []string{"EventsT"},
			goldenRel: "tests/fixtures/rust/eventspkg/events.go",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tmp, err := os.CreateTemp("", "btf2go-*.go")
			if err != nil {
				t.Fatal(err)
			}
			defer func() {
				if err := os.Remove(tmp.Name()); err != nil && !os.IsNotExist(err) {
					t.Errorf("remove temp file %s: %v", tmp.Name(), err)
				}
			}()
			if err := tmp.Close(); err != nil {
				t.Fatalf("close temp file %s: %v", tmp.Name(), err)
			}

			args := []string{"run", "./cmd/btf2go", "generate",
				"--elf", c.elfRel,
				"--pkg", c.pkg,
				"--out", tmp.Name(),
				"--no-map-types",
			}
			for _, ty := range c.types {
				args = append(args, "--type", ty)
			}
			cmd := exec.Command("go", args...)
			cmd.Dir = root
			if out, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("btf2go generate (%s): %v\n%s", c.name, err, out)
			}

			got, err := os.ReadFile(tmp.Name())
			if err != nil {
				t.Fatal(err)
			}
			golden := filepath.Join(root, c.goldenRel)
			if os.Getenv("UPDATE_GOLDEN") == "1" {
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				t.Logf("golden updated at %s", golden)
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden %s: %v", golden, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%s golden mismatch.\n--- want ---\n%s\n--- got ---\n%s",
					c.name, want, got)
			}
		})
	}
}
