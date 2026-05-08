package main

import (
	"fmt"
	"strings"
)

func fallback(s, alt string) string {
	if s == "" {
		return alt
	}
	return s
}

// RenderReport assembles a markdown report from one tier's worth of
// findings each. Stable ordering: tiers appear in the order they
// were passed; findings inside a tier appear in the order they were
// produced.
func RenderReport(info RunInfo, results []TierResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# btf2go validation report\n\n")
	fmt.Fprintf(&b, "Run ID: `%s`\n", info.ID)
	fmt.Fprintf(&b, "Generated: %s\n", info.Timestamp)
	dirty := ""
	if info.Btf2go.Dirty {
		dirty = " (dirty tree)"
	}
	tag := ""
	if info.Btf2go.Tag != "" {
		tag = " [" + info.Btf2go.Tag + "]"
	}
	fmt.Fprintf(&b, "btf2go: %s%s (commit %s)%s\n", info.Btf2go.Version, tag, info.Btf2go.Commit, dirty)
	fmt.Fprintf(&b, "Environment: %s on %s, %s/%s, kernel=%s, %s\n",
		info.Environment.Kind, info.Environment.Host,
		info.Environment.OS, info.Environment.Arch,
		fallback(info.Environment.Kernel, "n/a"), info.Environment.Go)
	fmt.Fprintf(&b, "Params: tiers=%s kernel=%v manifest=%s\n\n",
		strings.Join(info.Params.Tiers, ","), info.Params.Kernel, info.Params.Manifest)

	// Headline: aggregate pass count across all tiers.
	var totalPass, totalFail, totalSkip int
	for _, r := range results {
		for _, f := range r.Findings {
			switch f.Status {
			case StatusPass:
				totalPass++
			case StatusFail:
				totalFail++
			case StatusSkip:
				totalSkip++
			}
		}
	}
	fmt.Fprintf(&b, "## Headline\n\n")
	fmt.Fprintf(&b, "%d findings: **%d PASS**, **%d FAIL**, %d SKIP across %d tiers.\n\n",
		totalPass+totalFail+totalSkip, totalPass, totalFail, totalSkip, len(results))

	// Per-tier sections.
	for _, r := range results {
		// Tier values are stored as "T1", "T2", etc. Strip the leading "T"
		// so headings render as "## Tier 1" rather than "## Tier T1".
		tierLabel := strings.TrimPrefix(r.Tier, "T")
		fmt.Fprintf(&b, "## Tier %s\n\n", tierLabel)
		fmt.Fprintf(&b, "Pass rate: %.1f%% (%d findings)\n\n",
			r.PassRate()*100, len(r.Findings))
		for _, f := range r.Findings {
			fmt.Fprintf(&b, "### `%s` — %s\n\n", f.Project, f.Status)
			if f.Summary != "" {
				fmt.Fprintf(&b, "%s\n\n", f.Summary)
			}
			if f.SkipReason != "" {
				fmt.Fprintf(&b, "_Skipped: %s_\n\n", f.SkipReason)
			}
			if f.Detail != "" {
				fmt.Fprintf(&b, "```\n%s\n```\n\n", strings.TrimSpace(f.Detail))
			}
		}
	}
	return b.String()
}
