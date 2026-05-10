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
				"no BTF section in %s\n\n"+
					"common cause: bpf-linker version mismatches the system LLVM\n"+
					"(e.g. bpf-linker v0.10.3 needs LLVM-22; many Linux distros ship\n"+
					"LLVM-19). The build succeeds but produces a BTF-less ELF.\n\n"+
					"verify with:  readelf -S %q | grep BTF\n"+
					"if no .BTF appears, rebuild with a matching bpf-linker:\n"+
					"  cargo install bpf-linker --version <X>  # match `llvm-config --version`\n\n"+
					"original error: %w",
				path, path, err)
		}
		return nil, fmt.Errorf("load BTF from %s: %w", path, err)
	}
	return spec, nil
}
