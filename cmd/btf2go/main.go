package main

import (
	"fmt"
	"os"
	"runtime/debug"

	"github.com/spf13/cobra"

	"github.com/danigoland/btf2go/internal/align"
	"github.com/danigoland/btf2go/internal/btfparser"
	"github.com/danigoland/btf2go/internal/generator"
	"github.com/danigoland/btf2go/internal/traverse"
)

func main() {
	root := &cobra.Command{Use: "btf2go", Short: "Generate Go structs from BTF"}
	root.AddCommand(generateCmd())
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
