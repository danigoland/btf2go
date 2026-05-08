package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// archiveRun writes <reportsDir>/<id>.md and a sibling <id>.json
// metadata sidecar, then refreshes latest_report.md to point at the
// new report. Per-run sidecars (instead of one shared index.json)
// keep parallel runs from racing on a mutable file and let `ls
// *.json` serve as the index. Returns the absolute report path.
func archiveRun(reportsDir string, info RunInfo, report string) (string, error) {
	if err := os.MkdirAll(reportsDir, 0o755); err != nil {
		return "", fmt.Errorf("mkdir %s: %w", reportsDir, err)
	}

	reportName := info.ID + ".md"
	reportPath := filepath.Join(reportsDir, reportName)
	if err := os.WriteFile(reportPath, []byte(report), 0o644); err != nil {
		return "", fmt.Errorf("write report: %w", err)
	}

	info.Report = reportName
	meta, err := json.MarshalIndent(info, "", "  ")
	if err != nil {
		return "", fmt.Errorf("marshal sidecar: %w", err)
	}
	if err := os.WriteFile(filepath.Join(reportsDir, info.ID+".json"), append(meta, '\n'), 0o644); err != nil {
		return "", fmt.Errorf("write sidecar: %w", err)
	}

	if err := refreshSymlink(reportsDir, reportName); err != nil {
		return "", fmt.Errorf("update symlink: %w", err)
	}

	return reportPath, nil
}

// refreshSymlink points latest_report.md at the just-written report.
// Uses a relative target so it survives a `mv reports/` to another
// path. On platforms that fail symlink creation (e.g. Windows
// without dev-mode) we fall back to a copy.
func refreshSymlink(reportsDir, target string) error {
	link := filepath.Join(reportsDir, "latest_report.md")
	_ = os.Remove(link)
	if err := os.Symlink(target, link); err == nil {
		return nil
	}
	src := filepath.Join(reportsDir, target)
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(link, data, 0o644)
}
