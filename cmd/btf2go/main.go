package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "btf2go",
		Short: "Generate Go structs from BTF in compiled eBPF ELF artifacts",
	}
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
		RunE: func(c *cobra.Command, args []string) error {
			return fmt.Errorf("not implemented")
		},
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
