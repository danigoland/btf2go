package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

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

func runAll(cmd *cobra.Command, _ []string) error {
	tiers, _ := cmd.Flags().GetStringSlice("tier")
	wantKernel, _ := cmd.Flags().GetBool("kernel")
	manifestPath, _ := cmd.Flags().GetString("manifest")
	outPath, _ := cmd.Flags().GetString("out")

	m, err := LoadManifest(manifestPath)
	if err != nil {
		return err
	}
	corpusRoot := filepath.Dir(manifestPath)
	btf2goBin := "btf2go"
	if envBin := os.Getenv("BTF2GO_BIN"); envBin != "" {
		btf2goBin = envBin
	}

	allowed := map[string]bool{"all": true, "1": true, "2": true, "2.5": true, "3": true, "4": true}
	want := map[string]bool{}
	var unknown []string
	for _, t := range tiers {
		if !allowed[t] {
			unknown = append(unknown, t)
			continue
		}
		want[t] = true
	}
	if len(unknown) > 0 {
		return fmt.Errorf("unknown tier(s): %s (allowed: 1, 2, 2.5, 3, 4, all)", strings.Join(unknown, ", "))
	}
	all := want["all"]

	var results []TierResult
	if all || want["1"] {
		results = append(results, TierResult{Tier: "T1", Findings: RunTier1(m, corpusRoot, btf2goBin)})
	}
	if all || want["2"] {
		results = append(results, TierResult{Tier: "T2", Findings: RunTier2(m, corpusRoot, btf2goBin)})
	}
	if all || want["2.5"] {
		var findings []Finding
		if wantKernel {
			findings = RunTier2_5()
		} else {
			findings = []Finding{{Project: "T2.5-WireT", Status: StatusSkip,
				SkipReason: "T2.5 requires --kernel"}}
		}
		results = append(results, TierResult{Tier: "T2.5", Findings: findings})
	}
	if all || want["3"] {
		results = append(results, TierResult{Tier: "T3", Findings: RunTier3(m, corpusRoot, btf2goBin)})
	}
	if all || want["4"] {
		results = append(results, TierResult{Tier: "T4",
			Findings: RunTier4(filepath.Join(corpusRoot, "..", "runner", "ux", "transcript.md"))})
	}

	version, commit := toolVersionAndCommit()
	report := RenderReport(version, commit, results)
	if err := os.WriteFile(outPath, []byte(report), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d tiers, %d findings)\n", outPath, len(results), totalFindings(results))
	return nil
}

func totalFindings(rs []TierResult) int {
	var n int
	for _, r := range rs {
		n += len(r.Findings)
	}
	return n
}

// toolVersionAndCommit returns the btf2go version + a short git
// SHA so the report ties findings to a specific build of the tool.
func toolVersionAndCommit() (string, string) {
	commit := "unknown"
	if out, err := exec.Command("git", "-C", "..", "rev-parse", "--short", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	}
	return "v0.3.0", commit
}
