package main

import (
	"strings"
	"testing"
)

func TestRenderReportContainsHeadlines(t *testing.T) {
	results := []TierResult{
		{Tier: "T1", Findings: []Finding{
			{Project: "p1", Status: StatusPass, Summary: "5 of 5 structs match"},
			{Project: "p2", Status: StatusFail, Summary: "1 of 3 mismatches",
				Detail: "field Foo.bar offset = 4, want 8"},
		}},
		{Tier: "T2", Findings: []Finding{
			{Project: "elf-a", Status: StatusSkip, SkipReason: "clang not on PATH"},
		}},
	}
	info := RunInfo{
		ID:        "test-id",
		Timestamp: "2026-05-08T18:45:00Z",
		Btf2go:    Btf2goInfo{Version: "v0.3.0-test", Commit: "deadbeef"},
		Params:    ParamsInfo{Tiers: []string{"all"}, Manifest: "../corpus/manifest.yaml"},
		Environment: EnvironmentInfo{
			Kind: "local", Host: "test", OS: "darwin", Arch: "arm64", Go: "go1.25.5",
		},
	}
	out := RenderReport(info, results)
	for _, want := range []string{
		"# btf2go validation report",
		"v0.3.0-test",
		"deadbeef",
		"## Tier 1",
		"## Tier 2",
		"5 of 5 structs match",
		"clang not on PATH",
		"field Foo.bar offset = 4, want 8",
		// Headline rollup: 1 PASS + 1 FAIL + 1 SKIP = 3 findings
		// across 2 tiers. Asserting it explicitly catches rollup
		// math regressions that wouldn't change section content.
		"3 findings: **1 PASS**, **1 FAIL**, 1 SKIP across 2 tiers",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in report:\n%s", want, out)
		}
	}
}
