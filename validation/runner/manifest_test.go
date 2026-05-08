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

func TestLoadManifestRejectsUnknownKeys(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	// "outpattern" is a typo — strict decoder must error.
	data := `
c_corpus:
  - name: typo
    source_url: https://example.invalid
    build:
      cmd: ""
      outpattern: foo
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected error from unknown key 'outpattern', got nil")
	}
}

func TestLoadManifestRequiresName(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	data := `
c_corpus:
  - source_url: https://example.invalid
    pinned_commit: v1
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected error from missing name, got nil")
	}
}

func TestLoadManifestRequiresSourceURL(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	data := `
c_corpus:
  - name: no-url
    pinned_commit: v1
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected error from missing source_url, got nil")
	}
}

func TestLoadManifestRequiresPinnedCommit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "manifest.yaml")
	data := `
c_corpus:
  - name: no-pin
    source_url: https://example.invalid
`
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadManifest(path); err == nil {
		t.Fatal("expected error from missing pinned_commit, got nil")
	}
}
