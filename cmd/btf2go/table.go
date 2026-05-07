package main

import (
	"fmt"
	"io"
	"strings"
)

// table accumulates rows and prints them as a fixed-width left-aligned
// table once flushed. Used by `btf2go inspect`. Kept tiny on purpose —
// adding a tablewriter dependency for a single command would be silly.
type table struct {
	w     io.Writer
	rows  [][]string
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
