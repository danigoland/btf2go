package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"time"
)

// RunTier4 reads ux/transcript.md and surfaces its frontmatter as a
// Finding. The walkthrough itself is human-conducted; this just
// folds its outcome into the unified report.
func RunTier4(transcriptPath string) []Finding {
	f, err := os.Open(transcriptPath)
	if err != nil {
		return []Finding{{Project: "T4-UX", Status: StatusSkip,
			SkipReason: fmt.Sprintf("transcript missing: %v", err)}}
	}
	defer f.Close()

	meta := map[string]string{}
	sc := bufio.NewScanner(f)
	inFront := 0
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "---" {
			inFront++
			if inFront == 2 {
				break
			}
			continue
		}
		if inFront != 1 {
			continue
		}
		if i := strings.Index(line, ":"); i > 0 {
			k := strings.TrimSpace(line[:i])
			v := strings.TrimSpace(line[i+1:])
			meta[k] = v
		}
	}
	if err := sc.Err(); err != nil {
		return []Finding{{Project: "T4-UX", Status: StatusFail,
			Detail: fmt.Sprintf("transcript scan: %v", err)}}
	}
	if meta["status"] != "success" {
		return []Finding{{Project: "T4-UX", Status: StatusSkip,
			SkipReason: "transcript reports status=" + meta["status"]}}
	}
	start, errS := time.Parse(time.RFC3339, meta["start"])
	end, errE := time.Parse(time.RFC3339, meta["end"])
	if errS != nil || errE != nil {
		return []Finding{{Project: "T4-UX", Status: StatusFail,
			Detail: fmt.Sprintf("transcript timestamps unparseable: start=%v end=%v", errS, errE)}}
	}
	dur := end.Sub(start)
	if dur < 0 {
		return []Finding{{Project: "T4-UX", Status: StatusFail,
			Detail: fmt.Sprintf("transcript end (%s) is before start (%s)", meta["end"], meta["start"])}}
	}
	return []Finding{{Project: "T4-UX", Status: StatusPass,
		Summary: fmt.Sprintf("walkthrough completed in %s with %s friction point(s); see runner/ux/transcript.md",
			dur, meta["friction_points"])}}
}
