package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// RunInfo captures everything needed to identify and reproduce a run.
type RunInfo struct {
	ID          string         `json:"id"`
	Timestamp   string         `json:"timestamp"`
	Btf2go      Btf2goInfo     `json:"btf2go"`
	Params      ParamsInfo     `json:"params"`
	Environment EnvironmentInfo `json:"environment"`
	Headline    HeadlineInfo   `json:"headline"`
	Report      string         `json:"report"`
}

type Btf2goInfo struct {
	Version string `json:"version"`
	Commit  string `json:"commit"`
	Tag     string `json:"tag,omitempty"`
	Dirty   bool   `json:"dirty"`
}

type ParamsInfo struct {
	Tiers    []string `json:"tiers"`
	Kernel   bool     `json:"kernel"`
	Manifest string   `json:"manifest"`
}

type EnvironmentInfo struct {
	Kind     string `json:"kind"`     // local | daytona | proxmox | unknown
	Host     string `json:"host"`
	Kernel   string `json:"kernel,omitempty"`
	Go       string `json:"go"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
}

type HeadlineInfo struct {
	Pass  int `json:"pass"`
	Fail  int `json:"fail"`
	Skip  int `json:"skip"`
	Tiers int `json:"tiers"`
}

// gatherRunInfo collects all metadata for the current run. Called
// once at the start of runAll. The Headline + Report fields are
// filled in later, after tiers run.
func gatherRunInfo(tiers []string, kernel bool, manifest string) RunInfo {
	now := time.Now().UTC()
	commit := gitOutput("rev-parse", "--short", "HEAD")
	if commit == "" {
		commit = "unknown"
	}
	tag := gitOutput("describe", "--tags", "--exact-match", "HEAD")
	dirty := gitOutput("status", "--porcelain") != ""

	tierTag := tierTagForID(tiers)
	envKind := detectEnv()
	host, _ := os.Hostname()

	id := fmt.Sprintf("%s-%s-%s-%s",
		now.Format("2006-01-02T15-04-05Z"), commit, tierTag, envKind)

	return RunInfo{
		ID:        id,
		Timestamp: now.Format(time.RFC3339),
		Btf2go: Btf2goInfo{
			Version: "v0.3.0",
			Commit:  commit,
			Tag:     tag,
			Dirty:   dirty,
		},
		Params: ParamsInfo{
			Tiers:    append([]string(nil), tiers...),
			Kernel:   kernel,
			Manifest: manifest,
		},
		Environment: EnvironmentInfo{
			Kind:   envKind,
			Host:   host,
			Kernel: unameRelease(),
			Go:     runtime.Version(),
			OS:     runtime.GOOS,
			Arch:   runtime.GOARCH,
		},
	}
}

// tierTagForID renders the requested tier set as a stable filename
// fragment: ["all"] -> "all", ["1","2.5"] -> "t1+t2.5".
func tierTagForID(tiers []string) string {
	if len(tiers) == 1 && tiers[0] == "all" {
		return "all"
	}
	parts := make([]string, len(tiers))
	for i, t := range tiers {
		parts[i] = "t" + t
	}
	return strings.Join(parts, "+")
}

// detectEnv prefers VALIDATION_ENV (set by the orchestrator), and
// falls back to filesystem heuristics so a hand-run never lands in
// the index as the wrong kind.
func detectEnv() string {
	if v := os.Getenv("VALIDATION_ENV"); v != "" {
		return v
	}
	if _, err := os.Stat("/.dockerenv"); err == nil {
		return "daytona"
	}
	// Prefer an actual Proxmox-specific marker over generic BPF FS presence.
	if _, err := os.Stat("/etc/pve"); err == nil {
		return "proxmox"
	}
	if runtime.GOOS == "linux" {
		return "linux-host"
	}
	return "local"
}

func gitOutput(args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", ".."}, args...)...)
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func unameRelease() string {
	if runtime.GOOS != "linux" {
		return ""
	}
	out, err := exec.Command("uname", "-r").Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// MarshalIndent is a small wrapper so callers can write to disk
// without re-importing encoding/json.
func (r RunInfo) MarshalIndent() ([]byte, error) {
	return json.MarshalIndent(r, "", "  ")
}
