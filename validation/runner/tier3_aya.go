package main

import (
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunTier3 walks the aya_corpus and verifies that for each project's
// built ELF, btf2go can:
//  1. inspect (no crash on the BTF graph)
//  2. generate Go for every named struct
//  3. produce Go that compiles as a standalone package
//
// This is "does it work?" coverage, not layout-correctness — that's
// covered by T2 against the same ELFs if the user adds aya entries
// to the c_corpus too.
func RunTier3(m *Manifest, corpusRoot, btf2goBin string) []Finding {
	var out []Finding
	for _, p := range m.AyaCorpus {
		out = append(out, runTier3OneAya(p, corpusRoot, btf2goBin))
	}
	return out
}

func runTier3OneAya(p AyaProject, corpusRoot, btf2goBin string) Finding {
	projDir := filepath.Join(corpusRoot, "aya", p.Name)
	if _, err := os.Stat(projDir); err != nil {
		return Finding{Project: p.Name, Status: StatusSkip,
			SkipReason: "project not on disk — run refresh.sh"}
	}
	matches, _ := filepath.Glob(filepath.Join(projDir, p.Build.OutPattern))
	if len(matches) == 0 {
		return Finding{Project: p.Name, Status: StatusSkip,
			SkipReason: fmt.Sprintf("no ELF at %s — build failed in refresh.sh?", p.Build.OutPattern)}
	}

	var failures []string
	totalStructs := 0
	for _, elf := range matches {
		structs, err := runTier3OneELF(elf, btf2goBin)
		if err != nil {
			failures = append(failures, fmt.Sprintf("%s: %v", filepath.Base(elf), err))
			continue
		}
		totalStructs += structs
	}
	if len(failures) > 0 {
		return Finding{Project: p.Name, Status: StatusFail,
			Summary: fmt.Sprintf("%d of %d ELFs failed", len(failures), len(matches)),
			Detail:  strings.Join(failures, "\n")}
	}
	return Finding{Project: p.Name, Status: StatusPass,
		Summary: fmt.Sprintf("inspect + generate + compile pass across %d ELF(s), %d struct(s) total",
			len(matches), totalStructs)}
}

// runTier3OneELF runs the inspect/generate/compile cycle on one
// ELF and returns the count of structs covered or an error.
func runTier3OneELF(elf, btf2goBin string) (int, error) {
	if err := exec.Command(btf2goBin, "inspect", "--elf", elf).Run(); err != nil {
		return 0, fmt.Errorf("btf2go inspect crashed: %v", err)
	}
	expected, err := btfLayouts(elf)
	if err != nil {
		return 0, fmt.Errorf("btf load: %v", err)
	}
	if len(expected) == 0 {
		// Rust ELFs may legitimately strip mangled type names;
		// "no structs" is not a failure for this ELF.
		return 0, nil
	}

	tmp, err := os.MkdirTemp("", "btf2go-t3-")
	if err != nil {
		return 0, err
	}
	defer os.RemoveAll(tmp)
	pkgDir := filepath.Join(tmp, "gen")
	if err := os.Mkdir(pkgDir, 0o755); err != nil {
		return 0, err
	}
	outFile := filepath.Join(pkgDir, "gen.go")
	args := []string{"generate", "--elf", elf, "--pkg", "gen", "--out", outFile, "--no-map-types"}
	for name := range expected {
		args = append(args, "--type", name)
	}
	if o, err := exec.Command(btf2goBin, args...).CombinedOutput(); err != nil {
		return 0, fmt.Errorf("btf2go generate: %v\n%s", err, o)
	}
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module check\n\ngo "+goVersion()+"\n"), 0o644); err != nil {
		return 0, err
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = tmp
	if o, err := cmd.CombinedOutput(); err != nil {
		return 0, fmt.Errorf("compile generated Go: %v\n%s", err, o)
	}
	return len(expected), nil
}

// goVersion returns the major.minor of the Go toolchain (e.g.
// "1.25") for use in synthesized go.mod files. Falls back to a
// known-good baseline if introspection fails.
func goVersion() string {
	v := strings.TrimPrefix(build.Default.ReleaseTags[len(build.Default.ReleaseTags)-1], "go")
	if v == "" {
		return "1.22"
	}
	return v
}
