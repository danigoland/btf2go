# Regenerates eBPF .elf fixtures used by tests/golden_test.go and the
# layout verifiers. CI never runs this; CI runs pure Go against the
# committed .elf files. Maintainers run `make fixtures` (from the
# repo root) when they want to refresh the binaries after editing a
# fixture .c, .rs, or .zig source.
#
# Toolchain expectations:
#   - clang with BPF target support (Homebrew clang on macOS works
#     because no kernel headers are required for the type-only
#     fixtures; we don't compile programs that hit kernel APIs).
#
#   - rustc nightly with bpf-linker (for the rust/ subtree, future).
#   - zig (for the zig/ subtree, future).

CLANG ?= clang
ifeq ($(shell uname),Darwin)
  # Prefer Homebrew clang when available; the Apple-shipped clang
  # does not understand `-target bpf`.
  CLANG := $(firstword $(wildcard /opt/homebrew/opt/llvm/bin/clang) $(CLANG))
endif

.PHONY: all c clean

all: c

c: c/events.elf

c/events.elf: c/events.c
	$(CLANG) -target bpf -g -O2 -c $< -o $@

clean:
	rm -f c/events.elf
