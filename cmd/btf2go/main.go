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
	root := &cobra.Command{
		Use:     "btf2go",
		Short:   "Generate Go structs from BTF",
		Version: toolVersion(),
	}
	root.AddCommand(generateCmd(), inspectCmd(), versionCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func versionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print the btf2go version",
		RunE: func(cmd *cobra.Command, _ []string) error {
			_, err := fmt.Fprintln(cmd.OutOrStdout(), toolVersion())
			return err
		},
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
	cmd.Flags().Bool("aya", false, "enable aya HashMap<K,V> value-type unwrapping")
	cmd.Flags().StringArray("aya-bridge", nil, "custom bridge entry Name=arity:positions (repeatable); implies --aya")
	cmd.Flags().String("shared-out", "", "emit Pointer[T] and --shared-type entries to this file instead of inline")
	cmd.Flags().StringArray("shared-type", nil, "route a type to --shared-out instead of --out (repeatable; requires --shared-out)")
	cmd.Flags().String("source-name", "", "Override the // Source: header value (default: ELF basename). Use to keep generated files diff-stable across build hosts.")
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
	cmd.Flags().BoolP("verbose", "v", false, "expand DATASEC entries to show their vars and underlying types")
	cmd.Flags().Bool("names", false, "Show raw BTF name, Go-sanitized name, and terminal segment for each type")
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
	aya, _ := cmd.Flags().GetBool("aya")
	ayaBridgeRaw, _ := cmd.Flags().GetStringArray("aya-bridge")
	sharedOut, _ := cmd.Flags().GetString("shared-out")
	sharedTypes, _ := cmd.Flags().GetStringArray("shared-type")
	sourceName, _ := cmd.Flags().GetString("source-name")

	if len(ayaBridgeRaw) > 0 {
		aya = true // --aya-bridge implies --aya
	}

	bridgeOverrides := map[string]btfparser.BridgeSpec{}
	for _, raw := range ayaBridgeRaw {
		name, spec, err := btfparser.ParseBridgeOverride(raw)
		if err != nil {
			return err
		}
		bridgeOverrides[name] = spec
	}

	if sharedOut == "" && len(sharedTypes) > 0 {
		return fmt.Errorf("--shared-type requires --shared-out")
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
		Aya:           aya,
		AyaBridge:     bridgeOverrides,
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
		SourceName:  sourceName,
		ToolVersion: toolVersion(),
		SharedOut:   sharedOut,
		SharedTypes: sharedTypes,
	})
	// Always write what we have, even if gofmt failed.
	if writeErr := os.WriteFile(out, src, 0o644); writeErr != nil {
		return writeErr
	}
	if gErr != nil {
		fmt.Fprintf(os.Stderr, "warning: %v (unformatted output written to %s)\n", gErr, out)
	}
	fmt.Fprintf(os.Stderr, "Generated: %s\n", out)
	return nil
}

// inspectEntry is one row of the inspect output. Sorted by Kind, then
// Name, so the output is reproducible across runs.
type inspectEntry struct {
	Kind     string // STRUCT | UNION | ENUM | DATASEC
	Name     string
	Size     uint32
	Members  int
	Extra    string         // free-form ("signed" for signed enums, etc.)
	Children []datasecChild // populated for Datasec entries when --verbose
}

// datasecChild describes one variable inside a Datasec. Used only by
// the --verbose path of `btf2go inspect`.
type datasecChild struct {
	Name     string
	TypeKind string // STRUCT | UNION | ENUM | INT | etc.
	TypeName string // resolved underlying type name (for non-anonymous)
	Size     uint32
}

// datasecVars expands the Vars in a Datasec into datasecChild entries
// suitable for the --verbose listing.
func datasecVars(ds *btf.Datasec) []datasecChild {
	out := make([]datasecChild, 0, len(ds.Vars))
	for _, vsi := range ds.Vars {
		v, ok := vsi.Type.(*btf.Var)
		if !ok {
			continue
		}
		c := datasecChild{Name: v.Name, Size: vsi.Size}
		switch t := btf.UnderlyingType(v.Type).(type) {
		case *btf.Struct:
			c.TypeKind, c.TypeName = "STRUCT", t.Name
		case *btf.Union:
			c.TypeKind, c.TypeName = "UNION", t.Name
		case *btf.Enum:
			c.TypeKind, c.TypeName = "ENUM", t.Name
		case *btf.Int:
			c.TypeKind, c.TypeName = "INT", t.Name
		case *btf.Float:
			c.TypeKind, c.TypeName = "FLOAT", t.Name
		case *btf.Array:
			c.TypeKind = "ARRAY"
			if named, ok := t.Type.(interface{ TypeName() string }); ok {
				c.TypeName = fmt.Sprintf("[%d]%s", t.Nelems, named.TypeName())
			}
		case *btf.Pointer:
			c.TypeKind = "POINTER"
			pointee := btf.UnderlyingType(t.Target)
			if named, ok := pointee.(interface{ TypeName() string }); ok && named.TypeName() != "" {
				c.TypeName = "*" + named.TypeName()
			} else {
				c.TypeName = fmt.Sprintf("*%T", pointee)
			}
		default:
			c.TypeKind = fmt.Sprintf("%T", t)
		}
		out = append(out, c)
	}
	return out
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
	verbose, err := cmd.Flags().GetBool("verbose")
	if err != nil {
		return fmt.Errorf("read --verbose: %w", err)
	}
	filterLower := strings.ToLower(filter)

	spec, err := btfparser.Load(elf)
	if err != nil {
		return err
	}

	names, err := cmd.Flags().GetBool("names")
	if err != nil {
		return fmt.Errorf("read --names: %w", err)
	}
	if names {
		return renderNamesTable(cmd.OutOrStdout(), spec)
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
			if verbose {
				e.Children = datasecVars(v)
			}
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

	// Pretty-print as a fixed-width table. In --verbose mode, Datasec
	// rows are followed by indented var rows showing the underlying
	// type name, so the user can answer "what's in .rodata?" without
	// a separate dump tool.
	w := tablewriter(cmd.OutOrStdout())
	w.row("KIND", "NAME", "SIZE", "MEMBERS", "")
	for _, e := range entries {
		w.row(e.Kind, e.Name, fmt.Sprintf("%d", e.Size), fmt.Sprintf("%d", e.Members), e.Extra)
		for _, c := range e.Children {
			w.row("  └ VAR", c.Name, fmt.Sprintf("%d", c.Size), c.TypeKind, c.TypeName)
		}
	}
	return w.flush()
}

func toolVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		// debug.ReadBuildInfo returns "(devel)" for non-tagged local
		// builds; treat that as "no real version known" and fall back.
		if v := info.Main.Version; v != "" && v != "(devel)" {
			return v
		}
	}
	// TODO: inject real version via ldflags: -X main.version=vX.Y.Z
	return "v0.3.2"
}
