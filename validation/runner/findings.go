// Package main is the btf2go validation runner. Each tier reads
// the corpus manifest and emits []Finding. The aggregator in
// report.go assembles every tier's findings into a single report.
package main

import "fmt"

// Status is the result of one validation experiment per project.
type Status int

const (
	StatusPass Status = iota
	StatusFail
	StatusSkip
)

func (s Status) String() string {
	switch s {
	case StatusPass:
		return "PASS"
	case StatusFail:
		return "FAIL"
	case StatusSkip:
		return "SKIP"
	}
	return fmt.Sprintf("Status(%d)", int(s))
}

// Finding is the result of running one tier's experiment against
// one corpus project.
type Finding struct {
	// Project is the manifest entry name (e.g. "cilium/ebpf-tests"
	// or a synthetic name like "T2.5-WireT").
	Project string
	Status  Status
	// Summary is a one-line headline ("3 of 5 structs match").
	Summary string
	// Detail is per-finding free-form text shown in the report
	// under the project header. Multi-line OK; wrapped fenced.
	Detail string
	// SkipReason is non-empty only when Status == StatusSkip.
	// E.g. "rustc-nightly not on PATH" or "no /sys/fs/bpf".
	SkipReason string
}

// TierResult collects every Finding from one tier run plus a
// human-readable headline computed by the aggregator.
type TierResult struct {
	Tier     string // "T1", "T2", "T2.5", "T3", "T4"
	Findings []Finding
}

// PassRate returns the fraction of non-skip findings that passed.
// Returns 0 if every finding was skipped.
func (r TierResult) PassRate() float64 {
	var run, pass int
	for _, f := range r.Findings {
		if f.Status == StatusSkip {
			continue
		}
		run++
		if f.Status == StatusPass {
			pass++
		}
	}
	if run == 0 {
		return 0
	}
	return float64(pass) / float64(run)
}
