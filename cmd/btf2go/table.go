package main

import (
	"fmt"
	"io"
	"sort"
	"strings"

	"github.com/cilium/ebpf/btf"

	"github.com/danigoland/btf2go/internal/btfparser"
)

// table accumulates rows and prints them as a fixed-width left-aligned
// table once flushed. Used by `btf2go inspect`. Kept tiny on purpose —
// adding a tablewriter dependency for a single command would be silly.
type table struct {
	w      io.Writer
	rows   [][]string
	widths []int
}

func tablewriter(w io.Writer) *table {
	return &table{w: w}
}

func (t *table) row(cells ...string) {
	for i, c := range cells {
		if i >= len(t.widths) {
			t.widths = append(t.widths, len(c))
			continue
		}
		if len(c) > t.widths[i] {
			t.widths[i] = len(c)
		}
	}
	t.rows = append(t.rows, cells)
}

func (t *table) flush() error {
	for _, row := range t.rows {
		var b strings.Builder
		for i, c := range row {
			if i > 0 {
				b.WriteString("  ")
			}
			b.WriteString(c)
			// Pad to width unless this is the last cell.
			if i < len(row)-1 {
				for j := len(c); j < t.widths[i]; j++ {
					b.WriteByte(' ')
				}
			}
		}
		if _, err := fmt.Fprintln(t.w, strings.TrimRight(b.String(), " ")); err != nil {
			return err
		}
	}
	return nil
}

// renderNamesTable prints one row per named struct/union/enum with
// columns: kind | raw BTF name | Go-sanitized name | terminal segment.
// Users read off the right --type argument value from this output.
func renderNamesTable(w io.Writer, spec *btf.Spec) error {
	type row struct {
		kind, raw, sanitized, terminal string
	}
	var rows []row
	for t, err := range spec.All() {
		if err != nil {
			return err
		}
		var kind, raw string
		switch v := t.(type) {
		case *btf.Struct:
			if v.Name == "" {
				continue
			}
			kind, raw = "struct", v.Name
		case *btf.Union:
			if v.Name == "" {
				continue
			}
			kind, raw = "union", v.Name
		case *btf.Enum:
			if v.Name == "" {
				continue
			}
			kind, raw = "enum", v.Name
		default:
			continue
		}
		term := raw
		if idx := strings.LastIndex(raw, "::"); idx >= 0 {
			term = raw[idx+2:]
		}
		rows = append(rows, row{
			kind:      kind,
			raw:       raw,
			sanitized: btfparser.SanitizeName(raw),
			terminal:  term,
		})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].kind != rows[j].kind {
			return rows[i].kind < rows[j].kind
		}
		return rows[i].raw < rows[j].raw
	})
	fmt.Fprintf(w, "%-8s  %-40s  %-30s  %s\n", "kind", "raw", "go-ident", "terminal")
	for _, r := range rows {
		fmt.Fprintf(w, "%-8s  %-40s  %-30s  %s\n", r.kind, r.raw, r.sanitized, r.terminal)
	}
	return nil
}
