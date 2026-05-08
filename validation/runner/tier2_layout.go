package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/cilium/ebpf/btf"
	"github.com/danigoland/btf2go/internal/btfparser"
)

// btfLayouts loads BTF from an ELF and returns the kernel-truth
// layout for every named struct: size + field byte offsets.
//
// Field names are stored as raw BTF member names (not sanitized) so
// the unit test can query them with e.g. got["pid"]. RunTier2 applies
// btfparser.SanitizeName when comparing against generated Go output.
//
// Bitfield members are skipped — BTF reports them in bits and btf2go
// folds them into _bfN raw-byte storage blocks rather than individual
// addressable fields.
func btfLayouts(elfPath string) (map[string]goLayout, error) {
	spec, err := btf.LoadSpec(elfPath)
	if err != nil {
		return nil, fmt.Errorf("load BTF from %s: %w", elfPath, err)
	}
	out := map[string]goLayout{}
	for t, err := range spec.All() {
		if err != nil {
			return nil, fmt.Errorf("iterate BTF: %w", err)
		}
		s, ok := t.(*btf.Struct)
		if !ok || s.Name == "" {
			continue
		}
		layout := goLayout{
			Size:   int64(s.Size),
			Fields: map[string]int64{},
		}
		for _, m := range s.Members {
			if m.BitfieldSize > 0 {
				continue
			}
			// Store raw BTF name; RunTier2 sanitizes on comparison.
			layout.Fields[m.Name] = int64(m.Offset.Bytes())
		}
		out[s.Name] = layout
	}
	return out, nil
}

// RunTier2 cross-checks every named struct in every ELF in the C
// corpus against btf2go's generated layout. Each ELF produces one
// Finding aggregating per-struct results.
func RunTier2(m *Manifest, corpusRoot, btf2goBin string) []Finding {
	if _, err := exec.LookPath(btf2goBin); err != nil {
		return []Finding{{Project: "T2", Status: StatusSkip,
			SkipReason: fmt.Sprintf("btf2go binary not found at %s", btf2goBin)}}
	}
	var out []Finding
	for _, p := range m.CCorpus {
		projDir := filepath.Join(corpusRoot, "c", p.Name)
		matches, _ := filepath.Glob(filepath.Join(projDir, p.Build.OutPattern))
		if len(matches) == 0 {
			out = append(out, Finding{Project: p.Name, Status: StatusSkip,
				SkipReason: fmt.Sprintf("no ELF matched %s — did you run refresh.sh?", p.Build.OutPattern)})
			continue
		}
		for _, elf := range matches {
			out = append(out, runTier2OneELF(elf, p.Bpf2goPkg, btf2goBin))
		}
	}
	return out
}

// runTier2OneELF runs btf2go on a single ELF and diffs the generated
// Go layout against the BTF-truth layout extracted by btfLayouts.
func runTier2OneELF(elfPath, pkg, btf2goBin string) Finding {
	tag := elfPath

	expected, err := btfLayouts(elfPath)
	if err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("btf load: %v", err)}
	}
	if len(expected) == 0 {
		return Finding{Project: tag, Status: StatusSkip,
			SkipReason: "BTF has no named structs"}
	}

	tmp, err := os.MkdirTemp("", "btf2go-t2-")
	if err != nil {
		return Finding{Project: tag, Status: StatusFail, Detail: err.Error()}
	}
	defer os.RemoveAll(tmp)

	out := filepath.Join(tmp, "out.go")
	// btf2go's --type flag matches against raw BTF type names
	// (e.g. "events_t"), not the sanitized Go identifier.
	args := []string{"generate", "--elf", elfPath, "--pkg", pkg, "--out", out, "--no-map-types"}
	for name := range expected {
		args = append(args, "--type", name)
	}
	if cmdOut, err := exec.Command(btf2goBin, args...).CombinedOutput(); err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("btf2go: %v\n%s", err, cmdOut)}
	}

	got, err := parseGoLayouts(out)
	if err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("parse generated: %v", err)}
	}

	var diffs []string
	matched := 0
	for name, want := range expected {
		sanitized := btfparser.SanitizeName(name)
		g, ok := got[sanitized]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("%s: not in generated output", sanitized))
			continue
		}
		if g.Size != want.Size {
			diffs = append(diffs, fmt.Sprintf("%s size: got %d, want %d", sanitized, g.Size, want.Size))
			continue
		}
		fieldOK := true
		for fName, wantOff := range want.Fields {
			sanitizedField := btfparser.SanitizeName(fName)
			gotOff, ok := g.Fields[sanitizedField]
			if !ok {
				// btfLayouts already filters bitfield members out
				// of want.Fields, so a missing non-bitfield member
				// in the generated Go is a genuine coverage gap.
				diffs = append(diffs, fmt.Sprintf("%s.%s missing in generated output (BTF has it at offset %d)",
					sanitized, sanitizedField, wantOff))
				fieldOK = false
				continue
			}
			if gotOff != wantOff {
				diffs = append(diffs, fmt.Sprintf("%s.%s offset: got %d, want %d",
					sanitized, sanitizedField, gotOff, wantOff))
				fieldOK = false
			}
		}
		if fieldOK {
			matched++
		}
	}

	if len(diffs) == 0 {
		return Finding{Project: tag, Status: StatusPass,
			Summary: fmt.Sprintf("%d/%d structs match", matched, len(expected))}
	}
	return Finding{Project: tag, Status: StatusFail,
		Summary: fmt.Sprintf("%d/%d structs mismatch", len(expected)-matched, len(expected)),
		Detail:  strings.Join(diffs, "\n")}
}
