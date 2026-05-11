package btfparser

import (
	"errors"
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
		if errors.Is(err, btf.ErrNotFound) {
			return nil, fmt.Errorf(
				"ELF has no .BTF section: %s\n\n"+
					"For Rust/aya users:\n"+
					"  Add `-C link-arg=--btf` to rustflags in .cargo/config.toml under\n"+
					"  [target.bpfel-unknown-none]:\n\n"+
					"      [target.bpfel-unknown-none]\n"+
					"      rustflags = [\"-C\", \"link-arg=--btf\"]\n\n"+
					"  (bpf-linker requires --btf explicitly as of 0.10.3.)\n\n"+
					"For clang users:\n"+
					"  Compile with -g — clang's bpf target embeds BTF when debug info is on.\n\n"+
					"For zig users:\n"+
					"  See docs/aya-quickstart.md#zig-toolchain.\n\n"+
					"verify with:  readelf -S %q | grep BTF\n\n"+
					"original error: %w",
				path, path, err)
		}
		return nil, fmt.Errorf("load BTF from %s: %w", path, err)
	}
	return spec, nil
}
