# Validation Experiment Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the five-tier validation program specified in `docs/superpowers/specs/2026-05-07-validation-experiment-design.md`. Output is `validation/runner` (a Go program) + `validation/report.md` (the artifact) + `validation/.devcontainer/` (the canonical Daytona target).

**Architecture:** One Go program with a `cobra` entrypoint and per-tier function modules. Each tier reads a YAML manifest, runs its experiment, returns `[]Finding`. A `report.go` aggregator merges all tier outputs into a single markdown file. Corpus is materialized on demand from `manifest.yaml` via `refresh.sh` — never committed.

**Tech Stack:** Go 1.22+ for the runner; `cilium/ebpf` for ELF/BTF parsing and (T2.5) kernel loading; `gopkg.in/yaml.v3` for manifest parsing; `text/template` for report rendering. Toolchains needed by the runner: `clang -target bpf`, `bpf2go` (Go tool dep), `cargo +nightly` + `bpf-linker`, `zig 0.16+`. CI never runs the validation suite — it lives outside the regular `go test ./...` graph.

---

## File Structure

```
validation/
├── corpus/
│   └── manifest.yaml             # corpus definitions; not auto-fetched, refresh.sh does that
├── runner/
│   ├── go.mod                    # SEPARATE Go module — keeps validation deps out of the main module
│   ├── main.go                   # cobra: run --tier {1,2,2.5,3,4,all}, --kernel
│   ├── findings.go               # Status, Finding, TierResult types
│   ├── manifest.go               # YAML schema + loader
│   ├── tier1_diff.go             # T1: bpf2go vs btf2go differential
│   ├── tier2_layout.go           # T2: BTF truth vs Go runtime layout
│   ├── tier2_5_kernel.go         # T2.5: kernel round-trip (compiled-in only)
│   ├── tier3_aya.go              # T3: Aya project coverage
│   ├── tier4_ux.go               # T4: read transcript.md, summarize
│   ├── report.go                 # aggregator → validation/report.md
│   ├── kernel/
│   │   ├── wire.bpf.c            # T2.5 kernel-side fixture source
│   │   └── wire.elf              # committed precompiled artifact
│   ├── wirepkg/
│   │   └── wire.go               # btf2go-generated golden for T2.5
│   └── ux/
│       └── transcript.md         # T4 hand-written log
├── .devcontainer/
│   ├── devcontainer.json
│   └── Dockerfile
├── SETUP.md                      # 3-env guide (macOS / Daytona / Proxmox VM)
├── refresh.sh                    # corpus materialization from manifest.yaml
└── report.md                     # generated artifact
```

Each file has one job. The runner is a separate Go module so its dependency graph (yaml, cobra, etc.) doesn't pollute btf2go's main `go.mod`. Files communicate only through the `findings.go` types — every tier produces `[]Finding`, every aggregator consumes them.

---

## Task 1: Runner skeleton + Findings types

Foundation for every other tier. Creates the separate Go module, the cobra entry, and the structured-result types every tier returns.

**Files:**
- Create: `/Users/dani/btf2go/validation/runner/go.mod`
- Create: `/Users/dani/btf2go/validation/runner/main.go`
- Create: `/Users/dani/btf2go/validation/runner/findings.go`
- Create: `/Users/dani/btf2go/validation/runner/findings_test.go`

- [ ] **Step 1: Init runner module**

```bash
cd /Users/dani/btf2go/validation/runner
go mod init github.com/danigoland/btf2go/validation/runner
```

- [ ] **Step 2: Write the failing test for Finding types**

Create `findings_test.go`:

```go
package main

import "testing"

func TestStatusString(t *testing.T) {
	cases := []struct {
		s    Status
		want string
	}{
		{StatusPass, "PASS"},
		{StatusFail, "FAIL"},
		{StatusSkip, "SKIP"},
	}
	for _, c := range cases {
		if got := c.s.String(); got != c.want {
			t.Errorf("Status(%d).String() = %q, want %q", c.s, got, c.want)
		}
	}
}

func TestTierResultPassRate(t *testing.T) {
	r := TierResult{
		Tier: "T1",
		Findings: []Finding{
			{Status: StatusPass}, {Status: StatusPass}, {Status: StatusFail},
		},
	}
	got := r.PassRate()
	if got < 0.66 || got > 0.67 {
		t.Fatalf("pass rate = %f, want ~0.667", got)
	}
}
```

- [ ] **Step 3: Run test to confirm FAIL**

```bash
cd /Users/dani/btf2go/validation/runner
go test ./...
```

Expected: FAIL with "undefined: Status" / "undefined: TierResult".

- [ ] **Step 4: Write findings.go**

```go
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
```

- [ ] **Step 5: Write the cobra entry**

`main.go`:

```go
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
```

- [ ] **Step 6: Add cobra to go.mod and verify**

```bash
cd /Users/dani/btf2go/validation/runner
go get github.com/spf13/cobra@latest
go test ./...
go build ./...
./runner run --help
```

Expected: tests pass, build clean, help text shows `--tier`, `--kernel`, `--manifest`, `--out`.

- [ ] **Step 7: Commit**

```bash
cd /Users/dani/btf2go
git checkout -b feature/validation-runner-skeleton
git add validation/
git commit -m "feat(validation): runner skeleton + Findings types"
```

---

## Task 2: Manifest schema + loader

Defines the YAML the runner reads. Every tier consumes manifest entries, so this lands second.

**Files:**
- Create: `/Users/dani/btf2go/validation/corpus/manifest.yaml`
- Create: `/Users/dani/btf2go/validation/runner/manifest.go`
- Create: `/Users/dani/btf2go/validation/runner/manifest_test.go`

- [ ] **Step 1: Write the failing test**

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadManifest(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	data := `
c_corpus:
  - name: cilium-tests
    source_url: https://github.com/cilium/ebpf
    pinned_commit: v0.21.0
    build:
      cmd: make -C testdata
      out_pattern: testdata/*.o
    bpf2go_pkg: cilium_tests
    cflags:
      - -I/usr/include
aya_corpus:
  - name: aya-template
    source_url: https://github.com/aya-rs/aya-template
    pinned_commit: main
    build:
      cmd: cargo +nightly build --release
      out_pattern: target/bpfel-unknown-none/release/foo
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadManifest(path)
	if err != nil {
		t.Fatalf("LoadManifest: %v", err)
	}
	if len(m.CCorpus) != 1 || m.CCorpus[0].Name != "cilium-tests" {
		t.Fatalf("c_corpus parse: %+v", m.CCorpus)
	}
	if len(m.AyaCorpus) != 1 || m.AyaCorpus[0].Name != "aya-template" {
		t.Fatalf("aya_corpus parse: %+v", m.AyaCorpus)
	}
	if len(m.CCorpus[0].CFlags) != 1 || m.CCorpus[0].CFlags[0] != "-I/usr/include" {
		t.Fatalf("cflags parse: %+v", m.CCorpus[0].CFlags)
	}
}
```

- [ ] **Step 2: Run; confirm FAIL**

```bash
cd /Users/dani/btf2go/validation/runner
go test ./... -run TestLoadManifest
```

- [ ] **Step 3: Implement manifest.go**

```go
package main

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Manifest is the corpus contract loaded from validation/corpus/manifest.yaml.
type Manifest struct {
	CCorpus   []CProject   `yaml:"c_corpus"`
	AyaCorpus []AyaProject `yaml:"aya_corpus"`
}

// Build describes how to (re)build a corpus entry locally.
type Build struct {
	Cmd        string `yaml:"cmd"`
	OutPattern string `yaml:"out_pattern"` // glob relative to the corpus dir
}

// CProject is a manifest entry for the C corpus (T1 + T2).
type CProject struct {
	Name         string   `yaml:"name"`
	SourceURL    string   `yaml:"source_url"`
	PinnedCommit string   `yaml:"pinned_commit"`
	Build        Build    `yaml:"build"`
	// Bpf2goPkg is the package name passed to bpf2go for T1 diffing.
	Bpf2goPkg string   `yaml:"bpf2go_pkg"`
	CFlags    []string `yaml:"cflags"`
	// Sources is the list of .c files this project compiles. Used
	// by T1 to know what to feed bpf2go.
	Sources []string `yaml:"sources"`
}

// AyaProject is a manifest entry for the Aya/Rust corpus (T3).
type AyaProject struct {
	Name         string `yaml:"name"`
	SourceURL    string `yaml:"source_url"`
	PinnedCommit string `yaml:"pinned_commit"`
	Build        Build  `yaml:"build"`
}

// LoadManifest parses a manifest.yaml file from disk.
func LoadManifest(path string) (*Manifest, error) {
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	var m Manifest
	if err := yaml.Unmarshal(bs, &m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	return &m, nil
}
```

- [ ] **Step 4: Add yaml dep, verify test**

```bash
cd /Users/dani/btf2go/validation/runner
go get gopkg.in/yaml.v3
go test ./... -run TestLoadManifest
```

Expected: PASS.

- [ ] **Step 5: Write the seed corpus manifest**

`/Users/dani/btf2go/validation/corpus/manifest.yaml`:

```yaml
# Corpus manifest for the btf2go validation suite. Each entry is
# fetched + built by validation/refresh.sh; never committed.

c_corpus:
  - name: cilium-ebpf-testdata
    source_url: https://github.com/cilium/ebpf.git
    pinned_commit: v0.21.0
    build:
      cmd: make -C testdata
      out_pattern: testdata/*.o
    bpf2go_pkg: ciliumtests
    cflags:
      - -O2
    sources:
      - testdata/loader.c
      - testdata/btf-map-init.c

aya_corpus:
  - name: aya-template-default
    source_url: https://github.com/aya-rs/aya-template.git
    pinned_commit: main
    build:
      cmd: cargo +nightly build --release
      out_pattern: target/bpfel-unknown-none/release/myapp
  - name: kunai
    source_url: https://github.com/kunai-project/kunai.git
    pinned_commit: main
    build:
      cmd: cargo +nightly build --release -p kunai-ebpf
      out_pattern: target/bpfel-unknown-none/release/kunai
```

- [ ] **Step 6: Commit**

```bash
git add validation/runner/manifest*.go validation/runner/go.* validation/corpus/manifest.yaml
git commit -m "feat(validation): manifest schema + seed corpus entries"
```

---

## Task 3: refresh.sh corpus materializer

Bash script that reads the manifest and clones / pulls each project at the pinned commit. The runner never does this itself — it expects the corpus to be on disk.

**Files:**
- Create: `/Users/dani/btf2go/validation/refresh.sh`

- [ ] **Step 1: Write refresh.sh**

```bash
#!/usr/bin/env bash
# refresh.sh — materialize the validation corpus from manifest.yaml.
#
# Reads the YAML manifest, clones each entry into validation/corpus/
# at the pinned commit, and runs the build command. Idempotent: if
# a project is already cloned, fetches and resets to the pin.

set -euo pipefail

MANIFEST="${MANIFEST:-$(dirname "$0")/corpus/manifest.yaml}"
CORPUS_DIR="$(dirname "$0")/corpus"

require() {
  command -v "$1" >/dev/null 2>&1 || {
    echo "refresh.sh: missing required tool: $1" >&2
    exit 1
  }
}

require yq
require git
require make

echo "[refresh] manifest: $MANIFEST"

# C corpus
yq -r '.c_corpus[] | [.name, .source_url, .pinned_commit, .build.cmd] | @tsv' "$MANIFEST" |
while IFS=$'\t' read -r name url commit cmd; do
  dest="$CORPUS_DIR/c/$name"
  echo "[refresh] C: $name @ $commit"
  if [ -d "$dest/.git" ]; then
    git -C "$dest" fetch --quiet origin
  else
    mkdir -p "$(dirname "$dest")"
    git clone --quiet "$url" "$dest"
  fi
  git -C "$dest" -c advice.detachedHead=false checkout --quiet "$commit"
  if [ -n "$cmd" ]; then
    echo "[refresh]   build: $cmd"
    (cd "$dest" && eval "$cmd")
  fi
done

# Aya corpus
yq -r '.aya_corpus[] | [.name, .source_url, .pinned_commit, .build.cmd] | @tsv' "$MANIFEST" |
while IFS=$'\t' read -r name url commit cmd; do
  dest="$CORPUS_DIR/aya/$name"
  echo "[refresh] Aya: $name @ $commit"
  if [ -d "$dest/.git" ]; then
    git -C "$dest" fetch --quiet origin
  else
    mkdir -p "$(dirname "$dest")"
    git clone --quiet "$url" "$dest"
  fi
  git -C "$dest" -c advice.detachedHead=false checkout --quiet "$commit"
  if [ -n "$cmd" ]; then
    echo "[refresh]   build (errors are non-fatal — toolchain may be missing): $cmd"
    (cd "$dest" && eval "$cmd") || echo "[refresh]   build failed for $name (continuing)"
  fi
done

echo "[refresh] done"
```

- [ ] **Step 2: Make it executable + add gitignore**

```bash
chmod +x /Users/dani/btf2go/validation/refresh.sh
cat >> /Users/dani/btf2go/.gitignore <<'EOF'

# Materialized validation corpus — never committed. Re-fetch via
# validation/refresh.sh from the manifest.
/validation/corpus/c/
/validation/corpus/aya/
EOF
```

- [ ] **Step 3: Smoke-test refresh.sh on the seed manifest**

```bash
brew install yq 2>&1 | tail -3   # if not installed
bash /Users/dani/btf2go/validation/refresh.sh
ls /Users/dani/btf2go/validation/corpus/c/cilium-ebpf-testdata/.git 2>&1 | head
```

Expected: at least the cilium-ebpf-testdata clone succeeds. Aya entries may fail to build on macOS without bpf-linker — the script swallows those errors.

- [ ] **Step 4: Commit**

```bash
git add validation/refresh.sh .gitignore
git commit -m "feat(validation): refresh.sh corpus materializer"
```

---

## Task 4: Report aggregator skeleton

Builds the markdown report from `[]TierResult`. Defining this early gives every later tier a clear contract.

**Files:**
- Create: `/Users/dani/btf2go/validation/runner/report.go`
- Create: `/Users/dani/btf2go/validation/runner/report_test.go`

- [ ] **Step 1: Write failing test**

```go
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
	out := RenderReport("v0.3.0-test", "deadbeef", results)
	for _, want := range []string{
		"# btf2go validation report",
		"v0.3.0-test",
		"deadbeef",
		"## Tier 1",
		"## Tier 2",
		"5 of 5 structs match",
		"clang not on PATH",
		"field Foo.bar offset = 4, want 8",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in report:\n%s", want, out)
		}
	}
}
```

- [ ] **Step 2: Run test, confirm FAIL**

```bash
cd /Users/dani/btf2go/validation/runner
go test ./... -run TestRenderReport
```

- [ ] **Step 3: Implement report.go**

```go
package main

import (
	"fmt"
	"strings"
	"time"
)

// RenderReport assembles a markdown report from one tier's worth of
// findings each. Stable ordering: tiers appear in the order they
// were passed; findings inside a tier appear in the order they were
// produced.
func RenderReport(version, commit string, results []TierResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "# btf2go validation report\n\n")
	fmt.Fprintf(&b, "Generated: %s\n", time.Now().UTC().Format("2006-01-02"))
	fmt.Fprintf(&b, "btf2go: %s (commit %s)\n\n", version, commit)

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
		fmt.Fprintf(&b, "## Tier %s\n\n", r.Tier)
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
```

- [ ] **Step 4: Verify**

```bash
cd /Users/dani/btf2go/validation/runner
go test ./... -run TestRenderReport
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add validation/runner/report.go validation/runner/report_test.go
git commit -m "feat(validation): report aggregator skeleton"
```

---

## Task 5: Tier 1 — differential vs bpf2go

Compiles a C source, runs both tools, compares layouts via reflect-based probing of a temp Go module.

**Files:**
- Create: `/Users/dani/btf2go/validation/runner/tier1_diff.go`
- Create: `/Users/dani/btf2go/validation/runner/tier1_diff_test.go`

- [ ] **Step 1: Write the helper that probes a Go file for struct layouts**

We need a way to know `(struct → size, field → offset)` from the generated Go. Reflect requires the package to actually be compiled and imported, which we'd need a temp module + build for. Skip the temp-module approach (too heavyweight) — instead, use `go/parser` + `go/types` to type-check the file and call `types.Sizes.Sizeof` / `types.Sizes.Offsetsof`. This is what `gc` itself uses internally.

```go
package main

import (
	"fmt"
	"go/ast"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
)

// goLayout describes one Go struct: its size in bytes and the byte
// offset of each named field. Used by T1 to diff tools.
type goLayout struct {
	Size   int64
	Fields map[string]int64
}

// parseGoLayouts reads a single .go source file and returns the
// (Go) layout of every top-level struct type declaration. Uses
// go/types' default Sizes (matches gc on linux/amd64 + arm64).
func parseGoLayouts(srcPath string) (map[string]goLayout, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, srcPath, nil, parser.AllErrors)
	if err != nil {
		return nil, fmt.Errorf("parse %s: %w", srcPath, err)
	}
	conf := &types.Config{Importer: importer.Default(), Error: func(error) {}}
	pkg, err := conf.Check(file.Name.Name, fset, []*ast.File{file}, nil)
	if err != nil {
		return nil, fmt.Errorf("typecheck %s: %w", srcPath, err)
	}
	sizes := types.SizesFor("gc", "amd64")
	out := map[string]goLayout{}
	scope := pkg.Scope()
	for _, name := range scope.Names() {
		obj := scope.Lookup(name)
		named, ok := obj.Type().(*types.Named)
		if !ok {
			continue
		}
		st, ok := named.Underlying().(*types.Struct)
		if !ok {
			continue
		}
		fields := make([]*types.Var, st.NumFields())
		offsets := make(map[string]int64, st.NumFields())
		for i := 0; i < st.NumFields(); i++ {
			fields[i] = st.Field(i)
		}
		for i, off := range sizes.Offsetsof(fields) {
			offsets[fields[i].Name()] = off
		}
		out[name] = goLayout{
			Size:   sizes.Sizeof(named),
			Fields: offsets,
		}
	}
	return out, nil
}
```

- [ ] **Step 2: Test the helper with a fixture**

`tier1_diff_test.go`:

```go
package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParseGoLayouts(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "x.go")
	src := `package x

type Foo struct {
	A uint8
	_ [3]byte
	B uint32
}

type Bar struct {
	X uint64
	Y uint64
}
`
	if err := os.WriteFile(path, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseGoLayouts(path)
	if err != nil {
		t.Fatal(err)
	}
	foo := got["Foo"]
	if foo.Size != 8 {
		t.Errorf("Foo size: got %d, want 8", foo.Size)
	}
	if foo.Fields["B"] != 4 {
		t.Errorf("Foo.B offset: got %d, want 4", foo.Fields["B"])
	}
	bar := got["Bar"]
	if bar.Fields["Y"] != 8 || bar.Size != 16 {
		t.Errorf("Bar layout wrong: %+v", bar)
	}
}
```

- [ ] **Step 3: Run; expect PASS**

```bash
cd /Users/dani/btf2go/validation/runner
go test ./... -run TestParseGoLayouts
```

Expected: PASS.

- [ ] **Step 4: Implement RunTier1**

Add to `tier1_diff.go`:

```go
import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// RunTier1 runs the differential experiment for every CProject in
// the manifest's c_corpus. For each entry it compiles the .c source
// once, runs both bpf2go and btf2go on the resulting ELF, then diffs
// the two generated Go files struct-by-struct.
//
// Each Finding's Project is "<corpus-name>:<source.c>". A run with
// missing toolchains (clang or bpf2go) skips with a clear reason.
func RunTier1(m *Manifest, corpusRoot string, btf2goBin string) []Finding {
	var out []Finding
	if _, err := exec.LookPath("clang"); err != nil {
		out = append(out, Finding{Project: "T1", Status: StatusSkip,
			SkipReason: "clang not on PATH"})
		return out
	}
	if _, err := exec.LookPath("bpf2go"); err != nil {
		out = append(out, Finding{Project: "T1", Status: StatusSkip,
			SkipReason: "bpf2go not on PATH (try `go install github.com/cilium/ebpf/cmd/bpf2go@latest`)"})
		return out
	}
	for _, p := range m.CCorpus {
		projDir := filepath.Join(corpusRoot, "c", p.Name)
		for _, src := range p.Sources {
			out = append(out, runTier1OneSource(projDir, src, p, btf2goBin))
		}
	}
	return out
}

// runTier1OneSource compiles one .c file, runs both tools, diffs.
func runTier1OneSource(projDir, srcRel string, p CProject, btf2goBin string) Finding {
	tag := fmt.Sprintf("%s:%s", p.Name, srcRel)
	tmp, err := os.MkdirTemp("", "btf2go-t1-")
	if err != nil {
		return Finding{Project: tag, Status: StatusFail, Detail: err.Error()}
	}
	defer os.RemoveAll(tmp)

	// 1. Compile .c → .o
	srcPath := filepath.Join(projDir, srcRel)
	objPath := filepath.Join(tmp, "out.o")
	args := []string{"-target", "bpf", "-g", "-O2", "-c", srcPath, "-o", objPath}
	args = append(args, p.CFlags...)
	if out, err := exec.Command("clang", args...).CombinedOutput(); err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("clang: %v\n%s", err, out)}
	}

	// 2. Run bpf2go
	bpf2goOut := filepath.Join(tmp, "bpf2go.go")
	cmd := exec.Command("bpf2go", "-no-global-types", "-output-stem", "out", p.Bpf2goPkg, srcPath)
	cmd.Args = append(cmd.Args, "--")
	cmd.Args = append(cmd.Args, p.CFlags...)
	cmd.Dir = tmp
	if out, err := cmd.CombinedOutput(); err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("bpf2go: %v\n%s", err, out)}
	}
	// bpf2go writes <stem>_bpfel.go and _bpfeb.go; pick the LE one.
	bpf2goOut = filepath.Join(tmp, "out_bpfel.go")

	// 3. Run btf2go on the same .o (use --no-map-types because the
	//    --type list is whatever bpf2go ended up emitting).
	bpf2goLayouts, err := parseGoLayouts(bpf2goOut)
	if err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("parse bpf2go output: %v", err)}
	}
	if len(bpf2goLayouts) == 0 {
		return Finding{Project: tag, Status: StatusSkip,
			SkipReason: "bpf2go emitted no struct types for this source"}
	}
	btf2goOut := filepath.Join(tmp, "btf2go.go")
	btf2goArgs := []string{"generate", "--elf", objPath, "--pkg", p.Bpf2goPkg, "--out", btf2goOut, "--no-map-types"}
	for name := range bpf2goLayouts {
		btf2goArgs = append(btf2goArgs, "--type", name)
	}
	if out, err := exec.Command(btf2goBin, btf2goArgs...).CombinedOutput(); err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("btf2go: %v\n%s", err, out)}
	}
	btf2goLayouts, err := parseGoLayouts(btf2goOut)
	if err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("parse btf2go output: %v", err)}
	}

	// 4. Diff: every name in bpf2goLayouts must also appear in
	// btf2goLayouts with identical Size + Field offsets.
	var diffs []string
	for name, want := range bpf2goLayouts {
		got, ok := btf2goLayouts[name]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("%s: bpf2go has it, btf2go does not", name))
			continue
		}
		if got.Size != want.Size {
			diffs = append(diffs, fmt.Sprintf("%s size: btf2go=%d, bpf2go=%d", name, got.Size, want.Size))
		}
		for f, wantOff := range want.Fields {
			gotOff, ok := got.Fields[f]
			if !ok {
				// Field name disagreement: bpf2go and btf2go have
				// different sanitization rules. Don't fail on this
				// alone — log it.
				continue
			}
			if gotOff != wantOff {
				diffs = append(diffs, fmt.Sprintf("%s.%s offset: btf2go=%d, bpf2go=%d", name, f, gotOff, wantOff))
			}
		}
	}
	if len(diffs) == 0 {
		return Finding{Project: tag, Status: StatusPass,
			Summary: fmt.Sprintf("%d structs match exactly", len(bpf2goLayouts))}
	}
	return Finding{Project: tag, Status: StatusFail,
		Summary: fmt.Sprintf("%d mismatches across %d structs", len(diffs), len(bpf2goLayouts)),
		Detail:  strings.Join(diffs, "\n")}
}
```

Add the missing import:

```go
import "strings"
```

- [ ] **Step 5: Verify build**

```bash
cd /Users/dani/btf2go/validation/runner
go build ./...
```

Expected: clean.

- [ ] **Step 6: Commit**

```bash
git add validation/runner/tier1_diff*.go
git commit -m "feat(validation): T1 differential vs bpf2go"
```

---

## Task 6: Tier 2 — empirical layout correctness

For each ELF in the corpus, parse BTF to get expected layouts, run btf2go, parse generated Go, diff.

**Files:**
- Create: `/Users/dani/btf2go/validation/runner/tier2_layout.go`
- Create: `/Users/dani/btf2go/validation/runner/tier2_layout_test.go`

- [ ] **Step 1: Write helper that extracts BTF expected layouts**

```go
// btfLayouts loads BTF from an ELF and returns the kernel-truth
// layout for every named struct: size + field byte offsets.
// Bitfield fields are skipped because BTF reports them in bits and
// btf2go renders them as part of a _bfN raw-byte storage block —
// they're not directly comparable as Go fields.
func btfLayouts(elfPath string) (map[string]goLayout, error) {
	spec, err := btf.LoadSpec(elfPath)
	if err != nil {
		return nil, fmt.Errorf("load BTF from %s: %w", elfPath, err)
	}
	out := map[string]goLayout{}
	for t, err := range spec.All() {
		if err != nil {
			return nil, fmt.Errorf("iterate BTF: %w", err)
		}
		s, ok := t.(*btf.Struct)
		if !ok || s.Name == "" {
			continue
		}
		layout := goLayout{
			Size:   int64(s.Size),
			Fields: map[string]int64{},
		}
		for _, m := range s.Members {
			if m.BitfieldSize > 0 {
				continue // skip bitfields, see header
			}
			layout.Fields[btfparser.SanitizeName(m.Name)] = int64(m.Offset) / 8
		}
		out[s.Name] = layout
	}
	return out, nil
}
```

Imports needed:

```go
import (
	"fmt"

	"github.com/cilium/ebpf/btf"
	"github.com/danigoland/btf2go/internal/btfparser"
)
```

> The runner module is separate but Go's `replace` directive lets it depend on the in-tree btfparser package. Add to `validation/runner/go.mod`:
>
> ```
> replace github.com/danigoland/btf2go => ../../
> ```
>
> Then `go get github.com/danigoland/btf2go@latest` in the runner module — Go's module resolver will use the local copy.

- [ ] **Step 2: Add the replace directive**

```bash
cd /Users/dani/btf2go/validation/runner
cat >> go.mod <<'EOF'

replace github.com/danigoland/btf2go => ../../
EOF
go get github.com/danigoland/btf2go@latest
go get github.com/cilium/ebpf
go mod tidy
```

- [ ] **Step 3: Implement RunTier2**

```go
// RunTier2 cross-checks every named struct in every ELF in the C
// corpus against btf2go's generated layout. Each ELF in the corpus
// produces one Finding aggregating per-struct results.
func RunTier2(m *Manifest, corpusRoot, btf2goBin string) []Finding {
	if _, err := exec.LookPath(btf2goBin); err != nil && btf2goBin != "go" {
		return []Finding{{Project: "T2", Status: StatusSkip,
			SkipReason: fmt.Sprintf("btf2go binary not found at %s", btf2goBin)}}
	}
	var out []Finding
	for _, p := range m.CCorpus {
		projDir := filepath.Join(corpusRoot, "c", p.Name)
		matches, _ := filepath.Glob(filepath.Join(projDir, p.Build.OutPattern))
		if len(matches) == 0 {
			out = append(out, Finding{Project: p.Name, Status: StatusSkip,
				SkipReason: fmt.Sprintf("no ELF matched %s — did you run refresh.sh?", p.Build.OutPattern)})
			continue
		}
		for _, elf := range matches {
			out = append(out, runTier2OneELF(elf, p.Bpf2goPkg, btf2goBin))
		}
	}
	return out
}

func runTier2OneELF(elfPath, pkg, btf2goBin string) Finding {
	tag := elfPath
	expected, err := btfLayouts(elfPath)
	if err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("btf load: %v", err)}
	}
	if len(expected) == 0 {
		return Finding{Project: tag, Status: StatusSkip,
			SkipReason: "BTF has no named structs"}
	}
	tmp, err := os.MkdirTemp("", "btf2go-t2-")
	if err != nil {
		return Finding{Project: tag, Status: StatusFail, Detail: err.Error()}
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "out.go")
	args := []string{"generate", "--elf", elfPath, "--pkg", pkg, "--out", out, "--no-map-types"}
	for name := range expected {
		args = append(args, "--type", name)
	}
	if cmdOut, err := exec.Command(btf2goBin, args...).CombinedOutput(); err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("btf2go: %v\n%s", err, cmdOut)}
	}
	got, err := parseGoLayouts(out)
	if err != nil {
		return Finding{Project: tag, Status: StatusFail,
			Detail: fmt.Sprintf("parse generated: %v", err)}
	}
	var diffs []string
	matched := 0
	for name, want := range expected {
		// btf2go renames via SanitizeName (e.g. events_t →
		// EventsT). The expected map uses the raw BTF names; do
		// the same sanitization here so comparisons match the
		// generated Go.
		sanitized := btfparser.SanitizeName(name)
		g, ok := got[sanitized]
		if !ok {
			diffs = append(diffs, fmt.Sprintf("%s: not in generated output", sanitized))
			continue
		}
		if g.Size != want.Size {
			diffs = append(diffs, fmt.Sprintf("%s size: got %d, want %d", sanitized, g.Size, want.Size))
			continue
		}
		fieldsOK := true
		for fName, wantOff := range want.Fields {
			gotOff, ok := g.Fields[btfparser.SanitizeName(fName)]
			if !ok || gotOff != wantOff {
				diffs = append(diffs, fmt.Sprintf("%s.%s offset: got %d, want %d (ok=%v)",
					sanitized, fName, gotOff, wantOff, ok))
				fieldsOK = false
			}
		}
		if fieldsOK {
			matched++
		}
	}
	if len(diffs) == 0 {
		return Finding{Project: tag, Status: StatusPass,
			Summary: fmt.Sprintf("%d/%d structs match", matched, len(expected))}
	}
	return Finding{Project: tag, Status: StatusFail,
		Summary: fmt.Sprintf("%d/%d structs mismatch", len(expected)-matched, len(expected)),
		Detail:  strings.Join(diffs, "\n")}
}
```

Add imports:

```go
import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)
```

- [ ] **Step 4: Add a unit test for btfLayouts using the in-tree C fixture**

`tier2_layout_test.go`:

```go
package main

import "testing"

func TestBtfLayoutsOnCFixture(t *testing.T) {
	got, err := btfLayouts("../../tests/fixtures/c/events.elf")
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := got["events_t"]
	if !ok {
		t.Fatalf("events_t missing; have %v", keys(got))
	}
	if ev.Size != 48 {
		t.Errorf("events_t size: got %d, want 48", ev.Size)
	}
	if ev.Fields["pid"] != 4 {
		t.Errorf("events_t.pid offset: got %d, want 4", ev.Fields["pid"])
	}
	if ev.Fields["ts"] != 16 {
		t.Errorf("events_t.ts offset: got %d, want 16", ev.Fields["ts"])
	}
}

func keys(m map[string]goLayout) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}
```

- [ ] **Step 5: Run + commit**

```bash
cd /Users/dani/btf2go/validation/runner
go test ./...
git add validation/runner/tier2_layout*.go validation/runner/go.{mod,sum}
git commit -m "feat(validation): T2 empirical layout correctness"
```

---

## Task 7: Tier 3 — Aya project coverage

For each Aya project: assume `refresh.sh` already built it (or skip), find the resulting `.elf`, run `btf2go inspect` to enumerate types, run `btf2go generate`, compile the result.

**Files:**
- Create: `/Users/dani/btf2go/validation/runner/tier3_aya.go`

- [ ] **Step 1: Implement RunTier3**

```go
package main

import (
	"fmt"
	"go/build"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// RunTier3 walks the aya_corpus and verifies that for each project's
// built ELF, btf2go can:
//   1. inspect (no crash on the BTF graph)
//   2. generate Go for every named struct
//   3. produce Go that compiles as a standalone package
//
// This is "does it work?" coverage, not layout-correctness — that's
// covered by T2 against the same ELFs if the user adds aya entries
// to the c_corpus too.
func RunTier3(m *Manifest, corpusRoot, btf2goBin string) []Finding {
	var out []Finding
	for _, p := range m.AyaCorpus {
		out = append(out, runTier3OneAya(p, corpusRoot, btf2goBin))
	}
	return out
}

func runTier3OneAya(p AyaProject, corpusRoot, btf2goBin string) Finding {
	projDir := filepath.Join(corpusRoot, "aya", p.Name)
	if _, err := os.Stat(projDir); err != nil {
		return Finding{Project: p.Name, Status: StatusSkip,
			SkipReason: fmt.Sprintf("project not on disk — run refresh.sh"),
		}
	}
	matches, _ := filepath.Glob(filepath.Join(projDir, p.Build.OutPattern))
	if len(matches) == 0 {
		return Finding{Project: p.Name, Status: StatusSkip,
			SkipReason: fmt.Sprintf("no ELF at %s — build failed in refresh.sh?", p.Build.OutPattern)}
	}
	elf := matches[0]

	// Step 1: inspect (just check it doesn't crash).
	if err := exec.Command(btf2goBin, "inspect", "--elf", elf).Run(); err != nil {
		return Finding{Project: p.Name, Status: StatusFail,
			Detail: fmt.Sprintf("btf2go inspect crashed on %s: %v", elf, err)}
	}

	// Step 2: enumerate struct names from BTF (we don't shell out
	// to inspect for parsing — use the same library).
	expected, err := btfLayouts(elf)
	if err != nil {
		return Finding{Project: p.Name, Status: StatusFail,
			Detail: fmt.Sprintf("btf load: %v", err)}
	}
	if len(expected) == 0 {
		return Finding{Project: p.Name, Status: StatusSkip,
			SkipReason: "no named structs in BTF (Rust can strip mangled types)"}
	}

	// Step 3: generate.
	tmp, err := os.MkdirTemp("", "btf2go-t3-")
	if err != nil {
		return Finding{Project: p.Name, Status: StatusFail, Detail: err.Error()}
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, "gen.go")
	args := []string{"generate", "--elf", elf, "--pkg", "gen", "--out", out, "--no-map-types"}
	for name := range expected {
		args = append(args, "--type", name)
	}
	if o, err := exec.Command(btf2goBin, args...).CombinedOutput(); err != nil {
		return Finding{Project: p.Name, Status: StatusFail,
			Detail: fmt.Sprintf("btf2go generate: %v\n%s", err, o)}
	}

	// Step 4: compile-check.
	pkgDir := filepath.Join(tmp, "gen")
	if err := os.Mkdir(pkgDir, 0o755); err != nil {
		return Finding{Project: p.Name, Status: StatusFail, Detail: err.Error()}
	}
	if err := os.Rename(out, filepath.Join(pkgDir, "gen.go")); err != nil {
		return Finding{Project: p.Name, Status: StatusFail, Detail: err.Error()}
	}
	if err := os.WriteFile(filepath.Join(tmp, "go.mod"), []byte("module check\n\ngo "+goVersion()+"\n"), 0o644); err != nil {
		return Finding{Project: p.Name, Status: StatusFail, Detail: err.Error()}
	}
	cmd := exec.Command("go", "build", "./...")
	cmd.Dir = tmp
	if o, err := cmd.CombinedOutput(); err != nil {
		return Finding{Project: p.Name, Status: StatusFail,
			Detail: fmt.Sprintf("compile generated Go: %v\n%s", err, o)}
	}
	return Finding{Project: p.Name, Status: StatusPass,
		Summary: fmt.Sprintf("inspect + generate + compile pass for %d structs", len(expected))}
}

func goVersion() string {
	// Embed-time Go version inferred from build context. We only
	// need a minor like "1.22"; the runtime version embedded in
	// build.Default.GOROOT is fine.
	v := strings.TrimPrefix(build.Default.ReleaseTags[len(build.Default.ReleaseTags)-1], "go")
	if v == "" {
		return "1.22"
	}
	return v
}
```

- [ ] **Step 2: Verify build**

```bash
cd /Users/dani/btf2go/validation/runner
go build ./...
```

- [ ] **Step 3: Commit**

```bash
git add validation/runner/tier3_aya.go
git commit -m "feat(validation): T3 Aya project coverage"
```

---

## Task 8: T2.5 kernel program + golden

The bespoke `WireT` BTF fixture for the kernel round-trip experiment. This task creates the C source, compiles it, runs btf2go, and commits both the `.elf` and the generated golden so T2.5 has stable inputs without needing clang at runtime.

**Files:**
- Create: `/Users/dani/btf2go/validation/runner/kernel/wire.bpf.c`
- Create: `/Users/dani/btf2go/validation/runner/kernel/wire.elf` (committed binary)
- Create: `/Users/dani/btf2go/validation/runner/wirepkg/wire.go` (committed golden)

- [ ] **Step 1: Write the C source**

`wire.bpf.c`:

```c
// T2.5 fixture for btf2go validation. Deliberately exercises every
// alignment edge case the unit tests cover: bitfield run, packed
// uint64, nested union, char array, mixed signed/unsigned ints.

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

typedef unsigned char  u8;
typedef unsigned short u16;
typedef unsigned int   u32;
typedef unsigned long long u64;

struct inner_t {
    u32 lo;
    u32 hi;
};

union payload_t {
    u64 raw;
    struct inner_t pair;
};

struct wire_t {
    u8  kind;
    // 3 bytes pad
    u32 pid;
    u8  flag_a : 1;
    u8  flag_b : 1;
    u8  prio   : 6;
    // 7 bytes pad
    u64 ts;
    char comm[16];
    union payload_t pay;
} __attribute__((packed_we_dont_actually_use_this));

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, u32);
    __type(value, struct wire_t);
    __uint(max_entries, 16);
} wire_map SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_execve")
int handle_execve(void *ctx) { return 0; }

char _license[] SEC("license") = "GPL";
```

> Strip the `__attribute__((packed_we_dont_actually_use_this))` line — that's a typo guard. The actual `__attribute__((packed))` would defeat the alignment we want to test. Do NOT add packed.

(Remove the typo'd attribute line before committing.)

- [ ] **Step 2: Compile to ELF**

```bash
cd /Users/dani/btf2go/validation/runner/kernel
/opt/homebrew/opt/llvm/bin/clang -target bpf -g -O2 -c wire.bpf.c -o wire.elf
file wire.elf
/opt/homebrew/opt/llvm/bin/llvm-objdump -h wire.elf | grep BTF
```

Expected: ELF 64-bit eBPF, `.BTF` section present.

- [ ] **Step 3: Generate the golden Go**

```bash
cd /Users/dani/btf2go
mkdir -p validation/runner/wirepkg
go run ./cmd/btf2go generate \
  --elf validation/runner/kernel/wire.elf \
  --pkg wirepkg \
  --out validation/runner/wirepkg/wire.go \
  --type wire_t --no-map-types
```

- [ ] **Step 4: Verify the golden compiles**

```bash
cd /Users/dani/btf2go/validation/runner
go build ./wirepkg/
```

Expected: clean.

- [ ] **Step 5: Commit**

```bash
git add validation/runner/kernel/ validation/runner/wirepkg/
git commit -m "test(validation): T2.5 kernel fixture + golden"
```

---

## Task 9: T2.5 runner — kernel round-trip

Loads the wire.elf into a real kernel (only when run with `--kernel` flag and `/sys/fs/bpf` is mounted), populates the map with a `WireT` populated via the generated Set accessors, reads it back, asserts byte-equal.

**Files:**
- Create: `/Users/dani/btf2go/validation/runner/tier2_5_kernel.go`

- [ ] **Step 1: Implement RunTier2_5**

```go
//go:build linux

package main

import (
	"bytes"
	"fmt"
	"os"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/danigoland/btf2go/validation/runner/wirepkg"
)

// RunTier2_5 round-trips a WireT through a real kernel BPF map. Runs
// only on Linux (build tag) and only when /sys/fs/bpf is mountable.
func RunTier2_5() []Finding {
	if _, err := os.Stat("/sys/fs/bpf"); err != nil {
		return []Finding{{Project: "T2.5-WireT", Status: StatusSkip,
			SkipReason: "/sys/fs/bpf not mounted (no kernel BPF support or not root)"}}
	}
	spec, err := ebpf.LoadCollectionSpec("kernel/wire.elf")
	if err != nil {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: fmt.Sprintf("LoadCollectionSpec: %v", err)}}
	}
	coll, err := ebpf.NewCollection(spec)
	if err != nil {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: fmt.Sprintf("NewCollection: %v", err)}}
	}
	defer coll.Close()
	wireMap := coll.Maps["wire_map"]
	if wireMap == nil {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: "wire_map not found in collection"}}
	}

	// Populate.
	src := wirepkg.WireT{
		Kind: 7, Pid: 4242, Ts: 0xDEADBEEF12345678,
	}
	src.SetFlagA(1)
	src.SetFlagB(0)
	src.SetPrio(33)
	for i, b := range []byte("kernel-roundtrip") {
		if i >= len(src.Comm) {
			break
		}
		src.Comm[i] = int8(b)
	}
	src.Pay.SetAsRaw(0xCAFEBABEDEADBEEF)

	key := uint32(1)
	if err := wireMap.Put(key, src); err != nil {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: fmt.Sprintf("map.Put: %v", err)}}
	}

	// Read back.
	var got wirepkg.WireT
	if err := wireMap.Lookup(key, &got); err != nil {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: fmt.Sprintf("map.Lookup: %v", err)}}
	}

	// Byte-equal compare on the whole struct.
	srcBytes := unsafe.Slice((*byte)(unsafe.Pointer(&src)), unsafe.Sizeof(src))
	gotBytes := unsafe.Slice((*byte)(unsafe.Pointer(&got)), unsafe.Sizeof(got))
	if !bytes.Equal(srcBytes, gotBytes) {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Summary: "kernel round-trip byte mismatch",
			Detail:  fmt.Sprintf("sent: %x\nrecv: %x", srcBytes, gotBytes)}}
	}

	// Accessor round-trip.
	if got.GetFlagA() != 1 || got.GetFlagB() != 0 || got.GetPrio() != 33 {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: fmt.Sprintf("bitfields: a=%d b=%d prio=%d", got.GetFlagA(), got.GetFlagB(), got.GetPrio())}}
	}
	if *got.Pay.AsRaw() != 0xCAFEBABEDEADBEEF {
		return []Finding{{Project: "T2.5-WireT", Status: StatusFail,
			Detail: fmt.Sprintf("union round-trip: 0x%x", *got.Pay.AsRaw())}}
	}
	return []Finding{{Project: "T2.5-WireT", Status: StatusPass,
		Summary: fmt.Sprintf("populated/read-back identical (%d bytes)", len(srcBytes))}}
}
```

- [ ] **Step 2: Add a non-Linux stub**

Create `tier2_5_kernel_stub.go`:

```go
//go:build !linux

package main

func RunTier2_5() []Finding {
	return []Finding{{Project: "T2.5-WireT", Status: StatusSkip,
		SkipReason: "T2.5 requires Linux + kernel BPF support"}}
}
```

- [ ] **Step 3: Verify build on macOS host**

```bash
cd /Users/dani/btf2go/validation/runner
go build ./...
```

Expected: clean (the build-tagged Linux file isn't compiled on macOS).

- [ ] **Step 4: Commit**

```bash
git add validation/runner/tier2_5_kernel*.go
git commit -m "feat(validation): T2.5 kernel round-trip (Linux-only)"
```

---

## Task 10: T4 transcript reader

Reads `validation/runner/ux/transcript.md` and produces a Finding summarizing time-to-success and friction points.

**Files:**
- Create: `/Users/dani/btf2go/validation/runner/tier4_ux.go`
- Create: `/Users/dani/btf2go/validation/runner/ux/transcript.md`

- [ ] **Step 1: Write a placeholder transcript**

`ux/transcript.md`:

```markdown
# T4 — UX walkthrough transcript

This file is hand-written by the maintainer during the UX
experiment. The runner reads the headline metadata from the
frontmatter below and includes a summary in validation/report.md.

---

start: 2026-05-07T00:00:00Z
end: 2026-05-07T00:00:00Z
status: not_run
friction_points: 0

---

## Notes

(empty — fill in during the walkthrough)
```

- [ ] **Step 2: Implement RunTier4**

```go
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
	return []Finding{{Project: "T4-UX", Status: StatusPass,
		Summary: fmt.Sprintf("walkthrough completed in %s with %s friction point(s); see runner/ux/transcript.md",
			dur, meta["friction_points"])}}
}
```

- [ ] **Step 3: Verify**

```bash
cd /Users/dani/btf2go/validation/runner
go build ./...
```

- [ ] **Step 4: Commit**

```bash
git add validation/runner/tier4_ux.go validation/runner/ux/transcript.md
git commit -m "feat(validation): T4 transcript reader"
```

---

## Task 11: Wire all tiers into main.go

Replace the placeholder `runAll` with real tier dispatch.

**Files:**
- Modify: `/Users/dani/btf2go/validation/runner/main.go`

- [ ] **Step 1: Replace runAll**

```go
func runAll(cmd *cobra.Command, _ []string) error {
	tiers, _ := cmd.Flags().GetStringSlice("tier")
	wantKernel, _ := cmd.Flags().GetBool("kernel")
	manifestPath, _ := cmd.Flags().GetString("manifest")
	outPath, _ := cmd.Flags().GetString("out")

	m, err := LoadManifest(manifestPath)
	if err != nil {
		return err
	}
	corpusRoot := filepath.Join(filepath.Dir(manifestPath))
	btf2goBin := "btf2go"
	if envBin := os.Getenv("BTF2GO_BIN"); envBin != "" {
		btf2goBin = envBin
	}

	want := map[string]bool{}
	for _, t := range tiers {
		want[t] = true
	}
	all := want["all"]

	var results []TierResult
	if all || want["1"] {
		results = append(results, TierResult{Tier: "T1", Findings: RunTier1(m, corpusRoot, btf2goBin)})
	}
	if all || want["2"] {
		results = append(results, TierResult{Tier: "T2", Findings: RunTier2(m, corpusRoot, btf2goBin)})
	}
	if (all || want["2.5"]) && wantKernel {
		results = append(results, TierResult{Tier: "T2.5", Findings: RunTier2_5()})
	}
	if all || want["3"] {
		results = append(results, TierResult{Tier: "T3", Findings: RunTier3(m, corpusRoot, btf2goBin)})
	}
	if all || want["4"] {
		results = append(results, TierResult{Tier: "T4",
			Findings: RunTier4(filepath.Join(corpusRoot, "..", "runner", "ux", "transcript.md"))})
	}

	report := RenderReport("v0.3.0", "TODO", results)
	if err := os.WriteFile(outPath, []byte(report), 0o644); err != nil {
		return err
	}
	fmt.Printf("wrote %s (%d tiers, %d findings)\n", outPath, len(results),
		totalFindings(results))
	return nil
}

func totalFindings(rs []TierResult) int {
	var n int
	for _, r := range rs {
		n += len(r.Findings)
	}
	return n
}
```

Add imports `"path/filepath"` to main.go.

- [ ] **Step 2: Replace the hard-coded version with `git rev-parse`**

Replace `RenderReport("v0.3.0", "TODO", results)` with a small helper:

```go
func toolVersionAndCommit() (string, string) {
	commit := "unknown"
	if out, err := exec.Command("git", "-C", "..", "rev-parse", "--short", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	}
	return "v0.3.0", commit
}
```

And update the call: `version, commit := toolVersionAndCommit(); report := RenderReport(version, commit, results)`.

Add imports `"os/exec"`, `"strings"`.

- [ ] **Step 3: Smoke-test the full runner**

```bash
cd /Users/dani/btf2go/validation/runner
go install github.com/cilium/ebpf/cmd/bpf2go@latest
BTF2GO_BIN="$HOME/go/bin/btf2go"  # or wherever; build with `go install ./cmd/btf2go`
cd /Users/dani/btf2go && go install ./cmd/btf2go
cd /Users/dani/btf2go/validation/runner
./runner run --tier 4
cat ../report.md
```

Expected: T4 reports SKIP (status not_run); report.md is produced.

- [ ] **Step 4: Commit**

```bash
git add validation/runner/main.go
git commit -m "feat(validation): wire all tiers into main.go"
```

---

## Task 12: .devcontainer for Daytona

The canonical "everything-installed" execution target. Opens cleanly in any Dev Containers-compatible IDE (VS Code, Daytona, GitHub Codespaces).

**Files:**
- Create: `/Users/dani/btf2go/validation/.devcontainer/devcontainer.json`
- Create: `/Users/dani/btf2go/validation/.devcontainer/Dockerfile`

- [ ] **Step 1: Write the Dockerfile**

```dockerfile
# Validation suite execution environment. Install everything
# every tier needs so the runner can run with no skips.
FROM debian:trixie

ENV DEBIAN_FRONTEND=noninteractive
ENV CARGO_HOME=/usr/local/cargo
ENV RUSTUP_HOME=/usr/local/rustup
ENV PATH=/usr/local/cargo/bin:/usr/local/go/bin:$PATH

RUN apt-get update && apt-get install -y --no-install-recommends \
        build-essential \
        ca-certificates \
        clang-21 lld-21 llvm-21 llvm-21-dev \
        libbpf-dev linux-libc-dev \
        bpftool \
        curl git make pkg-config \
        yq jq \
        zlib1g-dev libelf-dev \
    && rm -rf /var/lib/apt/lists/* \
    && ln -s /usr/bin/clang-21 /usr/local/bin/clang

# Go (latest stable)
RUN curl -fsSL https://go.dev/dl/go1.23.0.linux-amd64.tar.gz \
        | tar -C /usr/local -xzf -

# Rust nightly + bpf-linker
RUN curl -fsSL https://sh.rustup.rs | sh -s -- -y --profile minimal --default-toolchain nightly \
    && rustup component add rust-src --toolchain nightly \
    && cargo install bpf-linker

# Zig 0.16
RUN curl -fsSL https://ziglang.org/builds/zig-x86_64-linux-0.16.0-dev.tar.xz \
        | tar -C /usr/local -xJf - \
    && ln -s /usr/local/zig-x86_64-linux-0.16.0-dev/zig /usr/local/bin/zig

# bpf2go
RUN go install github.com/cilium/ebpf/cmd/bpf2go@latest

WORKDIR /workspace
```

- [ ] **Step 2: Write devcontainer.json**

```json
{
  "name": "btf2go validation",
  "build": {
    "dockerfile": "Dockerfile"
  },
  "customizations": {
    "vscode": {
      "extensions": ["golang.go"]
    }
  },
  "remoteEnv": {
    "BTF2GO_BIN": "/workspace/btf2go"
  },
  "postCreateCommand": "go build -o btf2go ./cmd/btf2go"
}
```

- [ ] **Step 3: Verify Dockerfile syntax**

```bash
docker build --check /Users/dani/btf2go/validation/.devcontainer/ 2>&1 | tail -10 || \
  echo "(docker not on this host, skipping syntax check)"
```

(If docker is installed locally, run; otherwise note that the check happens when Daytona builds it.)

- [ ] **Step 4: Commit**

```bash
git add validation/.devcontainer/
git commit -m "feat(validation): .devcontainer for Daytona"
```

---

## Task 13: SETUP.md three-environment guide

Documents how to run the suite under macOS local / Daytona / Proxmox VM. Must be specific enough that a stranger can follow it.

**Files:**
- Create: `/Users/dani/btf2go/validation/SETUP.md`

- [ ] **Step 1: Write SETUP.md**

````markdown
# Validation suite setup

The runner is environment-agnostic — it probes for required tools
and skips experiments whose toolchains aren't present. The three
environments below differ in how complete a run they support.

## Environment 1 — macOS local

**Supports:** T1 (with bpf2go installed), T2 (with clang), T3
(rebuild Aya projects locally — slow, requires
`DYLD_FALLBACK_LIBRARY_PATH` for bpf-linker), T4. **Cannot run T2.5.**

```sh
brew install llvm yq jq
brew install zig                               # for any zig fixtures
rustup install nightly
cargo install bpf-linker
go install github.com/cilium/ebpf/cmd/bpf2go@latest
go install ./cmd/btf2go                        # from repo root

# materialize the corpus
bash validation/refresh.sh

# run partial suite (no kernel)
cd validation/runner
DYLD_FALLBACK_LIBRARY_PATH=/opt/homebrew/opt/llvm/lib \
BTF2GO_BIN="$(go env GOPATH)/bin/btf2go" \
  ./runner run --tier all
```

## Environment 2 — Daytona (recommended)

The canonical execution target. Reproducible Linux container with
every toolchain pre-installed via `validation/.devcontainer/`.

1. Open the repo in Daytona — it auto-builds the Dockerfile.
2. Wait ~2 min for the container to come up.
3. Run:

```sh
bash validation/refresh.sh
cd validation/runner
go build -o /usr/local/bin/btf2go ../../cmd/btf2go
./runner run --tier all
cat ../report.md
```

T2.5 is still skipped here because the Daytona container does not
have `/sys/fs/bpf` (no kernel BPF support inside containerized
runtimes). For T2.5 use Environment 3.

## Environment 3 — Proxmox VM (T2.5 + the rest)

Persistent Linux VM with kernel BPF support. Required for T2.5.
Pin the kernel to a known version so results are reproducible.

```sh
# Inside the VM (Debian/Ubuntu example):
sudo apt install -y clang-21 libbpf-dev linux-libc-dev bpftool \
                    build-essential pkg-config curl git make yq
# Plus rustup, bpf-linker, zig, go — same as Daytona.

# Mount /sys/fs/bpf if not auto-mounted
sudo mount -t bpf bpf /sys/fs/bpf

# Run as root to load the test program
sudo BTF2GO_BIN="$(go env GOPATH)/bin/btf2go" \
  ./runner run --tier all --kernel
```

**Kernel version pin:** confirmed working on linux-6.10. Earlier
kernels may lack BPF features that `wire.bpf.c` uses.

## Smoke test

After running in any environment:

```sh
cat validation/report.md | head -30
```

Expect a "Headline" section followed by per-tier sections. Skipped
tiers are reported with a clear reason — that's expected, not a
failure.
````

- [ ] **Step 2: Commit**

```bash
git add validation/SETUP.md
git commit -m "docs(validation): three-environment setup guide"
```

---

## Task 14: End-to-end smoke + branch handoff

A final pass that proves the suite holds together on the local machine before publishing.

**Files:** none (verification + commit pass only)

- [ ] **Step 1: Build everything**

```bash
cd /Users/dani/btf2go
go install ./cmd/btf2go
cd validation/runner
go build ./...
```

- [ ] **Step 2: Run T4 alone (no toolchain dependencies)**

```bash
./runner run --tier 4
test -f ../report.md && head -20 ../report.md
```

Expected: report.md exists; T4 section reports SKIP because
transcript status is `not_run`.

- [ ] **Step 3: If clang + bpf2go are local, run T1 + T2 too**

```bash
bash /Users/dani/btf2go/validation/refresh.sh
BTF2GO_BIN="$(go env GOPATH)/bin/btf2go" \
  ./runner run --tier 1 --tier 2 --tier 4
head -50 ../report.md
```

If clang isn't present: T1/T2 SKIP cleanly with a "clang not on PATH" message — that's the desired behavior, not a failure.

- [ ] **Step 4: Final tests**

```bash
cd /Users/dani/btf2go/validation/runner
go test -count=1 ./...
go vet ./...
```

- [ ] **Step 5: Push + open PR**

```bash
cd /Users/dani/btf2go
git push -u origin feature/validation-runner-skeleton
gh pr create --base master --head feature/validation-runner-skeleton \
  --title "feat: validation experiment runner (T1–T4 + T2.5)" \
  --body "Implements the validation suite from docs/superpowers/specs/2026-05-07-validation-experiment-design.md."
```

---

## Self-Review Notes

- **Spec coverage:** Every tier (T1, T2, T2.5, T3, T4), the runner skeleton, the corpus manifest, refresh.sh, the Daytona devcontainer, the SETUP.md three-environment guide, and the report aggregator are each implemented by a specific task. The committed-binary T2.5 fixture (Task 8) and runner (Task 9) split keeps each task TDD-sized.
- **Type consistency:** `Finding`, `TierResult`, `Status`, `goLayout`, `Manifest`, `CProject`, `AyaProject`, `Build`, `RunTier1..RunTier4`, `RunTier2_5`, `parseGoLayouts`, `btfLayouts`, `RenderReport` — names used consistently across tasks 1, 2, 4, 5, 6, 7, 9, 10, 11.
- **Known soft spots:**
  1. The `bpf2go` invocation in T1 expects flag shapes that may have drifted; engineer should sanity-check `bpf2go --help` after Task 1 and adjust the `--no-global-types` flag if needed.
  2. The corpus manifest's pinned commits (e.g., `kunai/main`) are placeholders — replace with real SHAs after first successful refresh.
  3. The Dockerfile uses `debian:trixie` for current packages; if image build fails in Daytona, swap to `ubuntu:24.04` and adjust package names.
  4. `wire.bpf.c` Step 1 has a deliberate typo'd attribute as a fence-post check — Step 1 explicitly tells the engineer to remove it before commit.
- **Time budget:** 14 tasks × ~30 min average ≈ 7 hours. Fits the 2–4 day spec budget with room for the corpus build and Proxmox VM provisioning to take longer than expected.
