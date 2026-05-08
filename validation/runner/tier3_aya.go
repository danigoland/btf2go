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
	elf := matches[0]

	if err := exec.Command(btf2goBin, "inspect", "--elf", elf).Run(); err != nil {
		return Finding{Project: p.Name, Status: StatusFail,
			Detail: fmt.Sprintf("btf2go inspect crashed on %s: %v", elf, err)}
	}

	expected, err := btfLayouts(elf)
	if err != nil {
		return Finding{Project: p.Name, Status: StatusFail,
			Detail: fmt.Sprintf("btf load: %v", err)}
	}
	if len(expected) == 0 {
		return Finding{Project: p.Name, Status: StatusSkip,
			SkipReason: "no named structs in BTF (Rust can strip mangled types)"}
	}

	tmp, err := os.MkdirTemp("", "btf2go-t3-")
	if err != nil {
		return Finding{Project: p.Name, Status: StatusFail, Detail: err.Error()}
	}
	defer os.RemoveAll(tmp)
	pkgDir := filepath.Join(tmp, "gen")
	if err := os.Mkdir(pkgDir, 0o755); err != nil {
		return Finding{Project: p.Name, Status: StatusFail, Detail: err.Error()}
	}
	outFile := filepath.Join(pkgDir, "gen.go")
	args := []string{"generate", "--elf", elf, "--pkg", "gen", "--out", outFile, "--no-map-types"}
	for name := range expected {
		args = append(args, "--type", name)
	}
	if o, err := exec.Command(btf2goBin, args...).CombinedOutput(); err != nil {
		return Finding{Project: p.Name, Status: StatusFail,
			Detail: fmt.Sprintf("btf2go generate: %v\n%s", err, o)}
	}

	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module check\n\ngo "+goVersion()+"\n"), 0o644); err != nil {
		return Finding{Project: p.Name, Status: StatusFail, Detail: err.Error()}
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = tmp
	if o, err := cmd.CombinedOutput(); err != nil {
		return Finding{Project: p.Name, Status: StatusFail,
			Detail: fmt.Sprintf("compile generated Go: %v\n%s", err, o)}
	}
	return Finding{Project: p.Name, Status: StatusPass,
		Summary: fmt.Sprintf("inspect + generate + compile pass for %d structs", len(expected))}
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
