package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

func main() {
	root := &cobra.Command{
		Use:   "validation-runner",
		Short: "Run the btf2go validation experiment suite",
	}
	root.AddCommand(runCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func runCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run one or all tiers and emit validation/report.md",
		RunE:  runAll,
	}
	cmd.Flags().StringSlice("tier", []string{"all"}, "tiers to run: 1, 2, 2.5, 3, 4, or all (repeatable)")
	cmd.Flags().Bool("kernel", false, "enable T2.5 (requires Linux + root + /sys/fs/bpf)")
	cmd.Flags().String("manifest", "../corpus/manifest.yaml", "path to corpus manifest")
	cmd.Flags().String("out", "../report.md", "path to write the aggregated report")
	return cmd
}

// runAll is the placeholder; real tier dispatch lands in later tasks.
func runAll(cmd *cobra.Command, _ []string) error {
	tiers, _ := cmd.Flags().GetStringSlice("tier")
	fmt.Println("would run tiers:", tiers)
	return nil
}
