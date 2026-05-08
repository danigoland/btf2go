package main

import (
	"bytes"
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

// LoadManifest parses a manifest.yaml file from disk. Unknown YAML
// keys cause a hard error so typos like "outpattern" don't silently
// produce empty Build entries that later trigger confusing skips.
func LoadManifest(path string) (*Manifest, error) {
	bs, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest %s: %w", path, err)
	}
	dec := yaml.NewDecoder(bytes.NewReader(bs))
	dec.KnownFields(true)
	var m Manifest
	if err := dec.Decode(&m); err != nil {
		return nil, fmt.Errorf("parse manifest %s: %w", path, err)
	}
	if err := m.validate(); err != nil {
		return nil, fmt.Errorf("manifest %s: %w", path, err)
	}
	return &m, nil
}

// validate enforces the minimum shape every tier relies on:
// each corpus entry must have a non-empty name and source URL.
// Build commands and out patterns may legitimately be empty.
func (m *Manifest) validate() error {
	for i, p := range m.CCorpus {
		if p.Name == "" {
			return fmt.Errorf("c_corpus[%d]: name is required", i)
		}
		if p.SourceURL == "" {
			return fmt.Errorf("c_corpus[%d] %q: source_url is required", i, p.Name)
		}
		if p.PinnedCommit == "" {
			return fmt.Errorf("c_corpus[%d] %q: pinned_commit is required", i, p.Name)
		}
	}
	for i, p := range m.AyaCorpus {
		if p.Name == "" {
			return fmt.Errorf("aya_corpus[%d]: name is required", i)
		}
		if p.SourceURL == "" {
			return fmt.Errorf("aya_corpus[%d] %q: source_url is required", i, p.Name)
		}
		if p.PinnedCommit == "" {
			return fmt.Errorf("aya_corpus[%d] %q: pinned_commit is required", i, p.Name)
		}
	}
	return nil
}
