package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRunTier4Missing(t *testing.T) {
	got := RunTier4(filepath.Join(t.TempDir(), "nope.md"))
	if len(got) != 1 || got[0].Status != StatusSkip {
		t.Fatalf("expected single SKIP for missing transcript, got %+v", got)
	}
}

func TestRunTier4Success(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.md")
	body := `# header

---

start: 2026-05-07T10:00:00Z
end: 2026-05-07T10:42:00Z
status: success
friction_points: 2

---

## Notes
free text after frontmatter is ignored
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := RunTier4(path)
	if len(got) != 1 || got[0].Status != StatusPass {
		t.Fatalf("expected PASS, got %+v", got)
	}
	if !strings.Contains(got[0].Summary, "42m") {
		t.Errorf("expected duration 42m in summary, got %q", got[0].Summary)
	}
	if !strings.Contains(got[0].Summary, "2 friction") && !strings.Contains(got[0].Summary, "2 ") {
		t.Errorf("expected friction count 2 in summary, got %q", got[0].Summary)
	}
}

func TestRunTier4StatusNotSuccess(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.md")
	body := `---
start: 2026-05-07T10:00:00Z
end: 2026-05-07T10:00:00Z
status: not_run
friction_points: 0
---
`
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got := RunTier4(path)
	if got[0].Status != StatusSkip || !strings.Contains(got[0].SkipReason, "not_run") {
		t.Fatalf("expected SKIP with not_run reason, got %+v", got)
	}
}
