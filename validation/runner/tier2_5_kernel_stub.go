//go:build !linux

package main

func RunTier2_5() []Finding {
	return []Finding{{Project: "T2.5-WireT", Status: StatusSkip,
		SkipReason: "T2.5 requires Linux + kernel BPF support"}}
}
