package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// goLayout describes one Go struct: its size in bytes and the byte
// offset of each named field. Used by T1 to diff tools.
type goLayout struct {
	Size   int64
	Fields map[string]int64
}

// stubImporter returns an empty stub package for any import path,
// so go/types can finish typechecking even when external packages
// aren't on the import graph (e.g. bpf2go output's cilium/ebpf
// reference). T1 only needs struct layout, which Sizes computes
// from the parsed field types — unresolved imports are tolerable.
type stubImporter struct{}

func (stubImporter) Import(path string) (*types.Package, error) {
	return types.NewPackage(path, path), nil
}

// parseGoLayouts reads a single .go source file and returns the
// (Go) layout of every top-level struct type declaration. Uses
// go/types' default Sizes (matches gc on linux/amd64 + arm64).
func parseGoLayouts(srcPath string) (map[string]goLayout, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, srcPath, nil, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", srcPath, err)
	}
	conf := &types.Config{Importer: stubImporter{}, Error: func(error) {}}
	pkg, err := conf.Check(file.Name.Name, fset, []*ast.File{file}, nil)
	if err != nil {
		return nil, fmt.Errorf("typecheck %s: %w", srcPath, err)
	}
	sizes := types.SizesFor("gc", "amd64")
	out := map[string]goLayout{}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		named, ok := obj.Type().(*types.Named)
		if !ok {
			continue
		}
		st, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}
		fields := make([]*types.Var, st.NumFields())
		offsets := make(map[string]int64, st.NumFields())
		for i := 0; i < st.NumFields(); i++ {
			fields[i] = st.Field(i)
		}
		for i, off := range sizes.Offsetsof(fields) {
			offsets[fields[i].Name()] = off
		}
		out[name] = goLayout{
			Size:   sizes.Sizeof(named),
			Fields: offsets,
		}
	}
	return out, nil
}

// RunTier1 runs the differential experiment for every CProject in
// the manifest's c_corpus. For each entry it compiles the .c source
// once, runs both bpf2go and btf2go on the resulting ELF, then diffs
// the two generated Go files struct-by-struct.
//
// Each Finding's Project is "<corpus-name>:<source.c>". A run with
// missing toolchains (clang or bpf2go) skips with a clear reason.
func RunTier1(m *Manifest, corpusRoot string, btf2goBin string) []Finding {
	var out []Finding
	if _, err := exec.LookPath("clang"); err != nil {
		out = append(out, Finding{Project: "T1", Status: StatusSkip,
			SkipReason: "clang not on PATH"})
		return out
	}
	if _, err := exec.LookPath("bpf2go"); err != nil {
		out = append(out, Finding{Project: "T1", Status: StatusSkip,
			SkipReason: "bpf2go not on PATH (try `go install github.com/cilium/ebpf/cmd/bpf2go@latest`)"})
		return out
	}
	for _, p := range m.CCorpus {
		projDir := filepath.Join(corpusRoot, "c", p.Name)
		for _, src := range p.Sources {
			out = append(out, runTier1OneSource(projDir, src, p, btf2goBin))
		}
	}
	return out
}

// runTier1OneSource compiles one .c file, runs both tools, diffs.
func runTier1OneSource(projDir, srcRel string, p CProject, btf2goBin string) Finding {
	tag := fmt.Sprintf("%s:%s", p.Name, srcRel)
	tmp, err := os.MkdirTemp("", "btf2go-t1-")
	if err != nil {
		return Finding{Project: tag, Status: StatusFail, Detail: err.Error()}
	}
	defer os.RemoveAll(tmp)

	srcPath, err := filepath.Abs(filepath.Join(projDir, srcRel))
	if err != nil {
		return Finding{Project: tag, Status: StatusFail, Detail: err.Error()}
	}
	objPath := filepath.Join(tmp, "out.o")
	args := []string{"-target", "bpf", "-g", "-O2", "-c", srcPath, "-o", objPath}
	args = append(args, p.CFlags...)
	if out, err := exec.Command("clang", args...).CombinedOutput(); err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("clang: %v\n%s", err, out)}
	}

	cmd := exec.Command("bpf2go", "-no-global-types", "-output-stem", "out", p.Bpf2goPkg, srcPath)
	cmd.Args = append(cmd.Args, "--")
	cmd.Args = append(cmd.Args, p.CFlags...)
	cmd.Dir = tmp
	// bpf2go is normally invoked via `go generate`, which sets
	// GOPACKAGE. Provide it explicitly so it works standalone.
	cmd.Env = append(os.Environ(), "GOPACKAGE="+p.Bpf2goPkg)
	if out, err := cmd.CombinedOutput(); err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("bpf2go: %v\n%s", err, out)}
	}
	bpf2goOut := filepath.Join(tmp, "out_bpfel.go")

	bpf2goLayouts, err := parseGoLayouts(bpf2goOut)
	if err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("parse bpf2go output: %v", err)}
	}
	if len(bpf2goLayouts) == 0 {
		return Finding{Project: tag, Status: StatusSkip,
			SkipReason: "bpf2go emitted no struct types for this source"}
	}

	btf2goOut := filepath.Join(tmp, "btf2go.go")
	btf2goArgs := []string{"generate", "--elf", objPath, "--pkg", p.Bpf2goPkg, "--out", btf2goOut, "--no-map-types"}
	for name := range bpf2goLayouts {
		btf2goArgs = append(btf2goArgs, "--type", name)
	}
	if out, err := exec.Command(btf2goBin, btf2goArgs...).CombinedOutput(); err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("btf2go: %v\n%s", err, out)}
	}
	btf2goLayouts, err := parseGoLayouts(btf2goOut)
	if err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("parse btf2go output: %v", err)}
	}

	var diffs []string
	for name, want := range bpf2goLayouts {
		got, ok := btf2goLayouts[name]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("%s: bpf2go has it, btf2go does not", name))
			continue
		}
		if got.Size != want.Size {
			diffs = append(diffs, fmt.Sprintf("%s size: btf2go=%d, bpf2go=%d", name, got.Size, want.Size))
		}
		for f, wantOff := range want.Fields {
			gotOff, ok := got.Fields[f]
			if !ok {
				continue
			}
			if gotOff != wantOff {
				diffs = append(diffs, fmt.Sprintf("%s.%s offset: btf2go=%d, bpf2go=%d", name, f, gotOff, wantOff))
			}
		}
	}
	if len(diffs) == 0 {
		return Finding{Project: tag, Status: StatusPass,
			Summary: fmt.Sprintf("%d structs match exactly", len(bpf2goLayouts))}
	}
	return Finding{Project: tag, Status: StatusFail,
		Summary: fmt.Sprintf("%d mismatches across %d structs", len(diffs), len(bpf2goLayouts)),
		Detail:  strings.Join(diffs, "\n")}
}
