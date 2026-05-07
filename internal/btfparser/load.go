package btfparser

import (
	"fmt"

	"github.com/cilium/ebpf/btf"
)

// Load reads BTF debug info from the given ELF file (or raw .btf file —
// btf.LoadSpec auto-detects). Returns an error if the file does not
// contain BTF; eBPF programs compiled without `clang -g` (or the rustc
// equivalent) will hit this.
func Load(path string) (*btf.Spec, error) {
	spec, err := btf.LoadSpec(path)
	if err != nil {
		return nil, fmt.Errorf("load BTF from %s: %w", path, err)
	}
	return spec, nil
}
