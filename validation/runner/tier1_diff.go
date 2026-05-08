package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
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

// parseGoLayouts reads a single .go source file and returns the
// layout (size + per-field offset) of every top-level struct type
// whose fields are entirely "simple": primitive integers/floats,
// fixed-size arrays of simple types, named primitive aliases (uintN,
// intN, byte, bool, etc), or recursive simple structs.
//
// Structs that reference external/qualified types (e.g.
// ebpf.MapSpec) or interface/func/map/chan types are skipped —
// they're wrapper types we don't compare. AST-only; no type-check.
func parseGoLayouts(srcPath string) (map[string]goLayout, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, srcPath, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", srcPath, err)
	}

	// First pass: collect all struct type declarations by name.
	structs := map[string]*ast.StructType{}
	for _, decl := range file.Decls {
		gd, ok := decl.(*ast.GenDecl)
		if !ok || gd.Tok != token.TYPE {
			continue
		}
		for _, spec := range gd.Specs {
			ts, ok := spec.(*ast.TypeSpec)
			if !ok {
				continue
			}
			st, ok := ts.Type.(*ast.StructType)
			if !ok {
				continue
			}
			structs[ts.Name.Name] = st
		}
	}

	out := map[string]goLayout{}
	for name, st := range structs {
		layout, ok := computeStructLayout(st, structs)
		if !ok {
			continue // contains non-simple field — skip
		}
		// Skip empty placeholder structs (e.g. bpf2go emits an
		// empty ciliumtestsVariableSpecs when the program has no
		// variables). They aren't BTF data types.
		if len(layout.Fields) == 0 {
			continue
		}
		out[name] = layout
	}
	return out, nil
}

// primitiveSize returns the size and alignment (in bytes) of a Go
// primitive type identifier on amd64/arm64, matching gc.
// Returns (0, 0, false) for unknown identifiers.
func primitiveSize(name string) (size, align int64, ok bool) {
	switch name {
	case "bool", "int8", "uint8", "byte":
		return 1, 1, true
	case "int16", "uint16":
		return 2, 2, true
	case "int32", "uint32", "float32", "rune":
		return 4, 4, true
	case "int64", "uint64", "float64", "complex64", "uintptr", "int", "uint":
		// int/uint are 8 on 64-bit (we only target amd64/arm64).
		// complex64 is 8 bytes (two float32s).
		return 8, 8, true
	case "complex128":
		return 16, 8, true
	}
	return 0, 0, false
}

// fieldSize returns the (size, alignment) of an AST type expression.
// Supports primitives, fixed-size arrays of supported types, pointer
// types (size 8), and references to other simple structs in the same
// file. Returns ok=false for anything else (selectors, maps, chans,
// funcs, interfaces, slices, ...).
func fieldSize(expr ast.Expr, structs map[string]*ast.StructType) (size, align int64, ok bool) {
	switch t := expr.(type) {
	case *ast.Ident:
		if size, align, ok := primitiveSize(t.Name); ok {
			return size, align, true
		}
		// Reference to another simple struct in the file?
		if st, found := structs[t.Name]; found {
			l, ok := computeStructLayout(st, structs)
			if !ok {
				return 0, 0, false
			}
			return l.Size, structAlign(st, structs), true
		}
		return 0, 0, false
	case *ast.ArrayType:
		// Fixed-size array only (Len != nil).
		if t.Len == nil {
			return 0, 0, false
		}
		n, ok := constInt(t.Len)
		if !ok {
			return 0, 0, false
		}
		elemSize, elemAlign, ok := fieldSize(t.Elt, structs)
		if !ok {
			return 0, 0, false
		}
		return n * elemSize, elemAlign, true
	case *ast.StarExpr:
		// Pointer is 8 bytes, but reject pointers to non-simple
		// pointees (selector exprs like *ebpf.MapSpec) so wrapper
		// structs don't pass the filter and get fed to btf2go.
		switch t.X.(type) {
		case *ast.Ident, *ast.ArrayType, *ast.StarExpr:
			return 8, 8, true
		}
		return 0, 0, false
	case *ast.IndexExpr:
		// Generic instantiation like Pointer[uint32].
		// btf2go emits exactly one generic: Pointer[T any] uint64,
		// which is always uint64-backed → 8 bytes, align 8.
		if id, ok := t.X.(*ast.Ident); ok && id.Name == "Pointer" {
			return 8, 8, true
		}
		return 0, 0, false
	}
	return 0, 0, false
}

// constInt extracts a non-negative int from an array length AST.
func constInt(e ast.Expr) (int64, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.INT {
		return 0, false
	}
	var n int64
	for _, r := range bl.Value {
		if r < '0' || r > '9' {
			return 0, false
		}
		n = n*10 + int64(r-'0')
	}
	return n, true
}

// structAlign returns the alignment of a (already-validated) simple
// struct: the max alignment among its fields, minimum 1.
func structAlign(st *ast.StructType, structs map[string]*ast.StructType) int64 {
	var a int64 = 1
	for _, f := range st.Fields.List {
		_, fa, ok := fieldSize(f.Type, structs)
		if !ok {
			continue
		}
		if fa > a {
			a = fa
		}
	}
	return a
}

// allBlank reports whether every identifier in names is the blank identifier "_".
func allBlank(names []*ast.Ident) bool {
	for _, n := range names {
		if n.Name != "_" {
			return false
		}
	}
	return true
}

// computeStructLayout walks st's fields, accumulating size with
// natural alignment padding, recording each named field's offset.
// Returns ok=false if any field is non-simple.
func computeStructLayout(st *ast.StructType, structs map[string]*ast.StructType) (goLayout, bool) {
	out := goLayout{Fields: map[string]int64{}}
	var off int64
	var maxAlign int64 = 1
	for _, f := range st.Fields.List {
		size, align, ok := fieldSize(f.Type, structs)
		if !ok {
			// If every name in this field declaration is the blank identifier "_",
			// treat it as a zero-size marker (e.g. bpf2go's `_ structs.HostLayout`)
			// and skip rather than aborting the struct.
			if len(f.Names) > 0 && allBlank(f.Names) {
				continue
			}
			return goLayout{}, false
		}
		if align > maxAlign {
			maxAlign = align
		}
		// Pad to alignment.
		if rem := off % align; rem != 0 {
			off += align - rem
		}
		// Record offset for each named field. Anonymous "_" still
		// consumes space but isn't recorded.
		for _, n := range f.Names {
			if n.Name == "_" {
				continue
			}
			out.Fields[n.Name] = off
		}
		// Anonymous embedded field (no Names): record by type name.
		if len(f.Names) == 0 {
			if id, ok := f.Type.(*ast.Ident); ok {
				out.Fields[id.Name] = off
			}
		}
		// Each name in the same field declaration occupies its own slot.
		nFields := int64(1)
		if len(f.Names) > 1 {
			nFields = int64(len(f.Names))
		}
		off += size * nFields
	}
	// Trailing pad to struct alignment.
	if rem := off % maxAlign; rem != 0 {
		off += maxAlign - rem
	}
	out.Size = off
	return out, true
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
	if btf2goBin == "" {
		out = append(out, Finding{Project: "T1", Status: StatusSkip,
			SkipReason: "btf2go binary path is empty"})
		return out
	}
	if _, err := exec.LookPath(btf2goBin); err != nil {
		out = append(out, Finding{Project: "T1", Status: StatusSkip,
			SkipReason: fmt.Sprintf("btf2go binary not found at %s", btf2goBin)})
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

	cmd := exec.Command("bpf2go", "-output-stem", "out", p.Bpf2goPkg, srcPath)
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

	expectedBTF, err := btfLayouts(objPath)
	if err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("btf load: %v", err)}
	}
	if len(bpf2goLayouts) == 0 || len(expectedBTF) == 0 {
		return Finding{Project: tag, Status: StatusSkip,
			SkipReason: "ELF has no BTF struct types"}
	}

	btf2goOut := filepath.Join(tmp, "btf2go.go")
	btf2goArgs := []string{"generate", "--elf", objPath, "--pkg", p.Bpf2goPkg, "--out", btf2goOut, "--no-map-types"}
	for name := range expectedBTF {
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
				diffs = append(diffs, fmt.Sprintf("%s.%s missing in btf2go output (bpf2go has it at offset %d)", name, f, wantOff))
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
