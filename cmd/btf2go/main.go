// Command btf2go is the CLI entrypoint for the BTF→Go struct
// generator. See README.md for usage.
package main

import (
	"fmt"
	"go/token"
	"os"
	"regexp"
	"runtime/debug"
	"sort"
	"strings"

	"github.com/cilium/ebpf/btf"
	"github.com/spf13/cobra"

	"github.com/danigoland/btf2go/internal/align"
	"github.com/danigoland/btf2go/internal/btfparser"
	"github.com/danigoland/btf2go/internal/generator"
	"github.com/danigoland/btf2go/internal/traverse"
)

// goPackageName is a strict subset of valid Go package names: a
// lowercase letter or underscore start, followed by lowercase letters,
// digits, or underscores. Go itself permits more (uppercase, unicode
// letters), but real-world packages stay inside this subset and
// matching it gives a clear up-front error for obvious mistakes like
// "events; rm -rf /" instead of waiting for `go build` to fail later.
var goPackageName = regexp.MustCompile(`^[a-z_][a-z0-9_]*$`)

func main() {
	root := &cobra.Command{Use: "btf2go", Short: "Generate Go structs from BTF"}
	root.AddCommand(generateCmd(), inspectCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "generate",
		Short: "Generate Go types from a BTF-bearing ELF",
		RunE:  runGenerate,
	}
	cmd.Flags().String("elf", "", "path to eBPF ELF artifact (required)")
	cmd.Flags().String("pkg", "", "Go package name for generated file (required)")
	cmd.Flags().String("out", "", "output .go file path (required)")
	cmd.Flags().StringSlice("type", nil, "explicit type to include (repeatable)")
	cmd.Flags().Bool("no-map-types", false, "skip auto-include of map key/value types")
	_ = cmd.MarkFlagRequired("elf")
	_ = cmd.MarkFlagRequired("pkg")
	_ = cmd.MarkFlagRequired("out")
	return cmd
}

func inspectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "inspect",
		Short: "List the BTF types in an ELF without generating Go code",
		Long: `inspect reads the BTF graph from a compiled eBPF ELF and lists every
named struct, union, enum, and datasec it finds, with size and member
counts. Useful when --type can't resolve a name and you want to see
what's actually in the file.`,
		RunE: runInspect,
	}
	cmd.Flags().String("elf", "", "path to eBPF ELF artifact (required)")
	cmd.Flags().String("filter", "", "case-insensitive substring filter on type names")
	_ = cmd.MarkFlagRequired("elf")
	return cmd
}

func runGenerate(cmd *cobra.Command, _ []string) error {
	elf, err := cmd.Flags().GetString("elf")
	if err != nil {
		return fmt.Errorf("read --elf: %w", err)
	}
	pkg, err := cmd.Flags().GetString("pkg")
	if err != nil {
		return fmt.Errorf("read --pkg: %w", err)
	}
	out, err := cmd.Flags().GetString("out")
	if err != nil {
		return fmt.Errorf("read --out: %w", err)
	}
	typeNames, err := cmd.Flags().GetStringSlice("type")
	if err != nil {
		return fmt.Errorf("read --type: %w", err)
	}
	noMaps, err := cmd.Flags().GetBool("no-map-types")
	if err != nil {
		return fmt.Errorf("read --no-map-types: %w", err)
	}

	if !goPackageName.MatchString(pkg) {
		return fmt.Errorf("--pkg %q is not a valid Go package name (expected ^[a-z_][a-z0-9_]*$)", pkg)
	}
	if token.IsKeyword(pkg) {
		return fmt.Errorf("--pkg %q is a reserved Go keyword", pkg)
	}

	spec, err := btfparser.Load(elf)
	if err != nil {
		return err
	}
	resolved, err := btfparser.Resolve(spec, btfparser.ResolveOptions{
		ExplicitTypes: typeNames,
		IncludeMaps:   !noMaps,
	})
	if err != nil {
		return err
	}
	ir, err := traverse.Build(pkg, resolved)
	if err != nil {
		return err
	}
	for i := range ir.Structs {
		if err := align.Apply(&ir.Structs[i]); err != nil {
			return fmt.Errorf("align %s: %w", ir.Structs[i].Name, err)
		}
	}
	src, gErr := generator.Generate(ir, generator.Options{
		Source:      elf,
		ToolVersion: toolVersion(),
	})
	// Always write what we have, even if gofmt failed.
	if writeErr := os.WriteFile(out, src, 0o644); writeErr != nil {
		return writeErr
	}
	if gErr != nil {
		fmt.Fprintf(os.Stderr, "warning: %v (unformatted output written to %s)\n", gErr, out)
	}
	return nil
}

// inspectEntry is one row of the inspect output. Sorted by Kind, then
// Name, so the output is reproducible across runs.
type inspectEntry struct {
	Kind     string // STRUCT | UNION | ENUM | DATASEC
	Name     string
	Size     uint32
	Members  int
	Extra    string // free-form ("signed" for signed enums, ".maps datasec name", etc.)
}

func runInspect(cmd *cobra.Command, _ []string) error {
	elf, err := cmd.Flags().GetString("elf")
	if err != nil {
		return fmt.Errorf("read --elf: %w", err)
	}
	filter, err := cmd.Flags().GetString("filter")
	if err != nil {
		return fmt.Errorf("read --filter: %w", err)
	}
	filterLower := strings.ToLower(filter)

	spec, err := btfparser.Load(elf)
	if err != nil {
		return err
	}

	var entries []inspectEntry
	for t, err := range spec.All() {
		if err != nil {
			return fmt.Errorf("iterating BTF spec: %w", err)
		}
		var e inspectEntry
		switch v := t.(type) {
		case *btf.Struct:
			e = inspectEntry{Kind: "STRUCT", Name: v.Name, Size: v.Size, Members: len(v.Members)}
		case *btf.Union:
			e = inspectEntry{Kind: "UNION", Name: v.Name, Size: v.Size, Members: len(v.Members)}
		case *btf.Enum:
			extra := "unsigned"
			if v.Signed {
				extra = "signed"
			}
			e = inspectEntry{Kind: "ENUM", Name: v.Name, Size: v.Size, Members: len(v.Values), Extra: extra}
		case *btf.Datasec:
			e = inspectEntry{Kind: "DATASEC", Name: v.Name, Size: v.Size, Members: len(v.Vars)}
		default:
			continue
		}
		// Skip anonymous types — they're noise in the listing.
		if e.Name == "" {
			continue
		}
		if filterLower != "" && !strings.Contains(strings.ToLower(e.Name), filterLower) {
			continue
		}
		entries = append(entries, e)
	}

	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Kind != entries[j].Kind {
			return entries[i].Kind < entries[j].Kind
		}
		return entries[i].Name < entries[j].Name
	})

	if len(entries) == 0 {
		if filter != "" {
			fmt.Fprintf(os.Stderr, "no named types in %s match filter %q\n", elf, filter)
		} else {
			fmt.Fprintf(os.Stderr, "no named struct/union/enum/datasec types in %s\n", elf)
		}
		return nil
	}

	// Pretty-print as a fixed-width table.
	w := tablewriter(cmd.OutOrStdout())
	w.row("KIND", "NAME", "SIZE", "MEMBERS", "")
	for _, e := range entries {
		w.row(e.Kind, e.Name, fmt.Sprintf("%d", e.Size), fmt.Sprintf("%d", e.Members), e.Extra)
	}
	w.flush()
	return nil
}

func toolVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		// debug.ReadBuildInfo returns "(devel)" for non-tagged local
		// builds; treat that as "no real version known" and fall back.
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	return "v0.1.0-dev"
}
