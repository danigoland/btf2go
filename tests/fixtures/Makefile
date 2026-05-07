# Regenerates eBPF .elf fixtures used by tests/golden_test.go and the
# layout verifiers. CI never runs this; CI runs pure Go against the
# committed .elf files. Maintainers run `make fixtures` (from the
# repo root) when they want to refresh the binaries after editing a
# fixture .c, .rs, or .zig source.
#
# Toolchain expectations:
#
#   c — clang with BPF target support (Homebrew clang on macOS works
#       because no kernel headers are required for the type-only
#       fixtures; we don't compile programs that hit kernel APIs).
#
#   rust — rustc nightly + bpf-linker. On macOS, set
#          DYLD_FALLBACK_LIBRARY_PATH to your Homebrew LLVM lib dir
#          so bpf-linker's LLVM proxy can find a matching libLLVM
#          for the rustc nightly toolchain. The recipe below sets
#          this automatically for the standard Homebrew install.
#
#   zig — (future) zig with BPF target support.

CLANG ?= clang
ifeq ($(shell uname),Darwin)
  # Prefer Homebrew clang when available; the Apple-shipped clang
  # does not understand `-target bpf`.
  CLANG := $(firstword $(wildcard /opt/homebrew/opt/llvm/bin/clang) $(CLANG))
  HOMEBREW_LLVM_LIB := /opt/homebrew/opt/llvm/lib
endif

CARGO ?= cargo

.PHONY: all c rust clean

all: c rust

c: c/events.elf

c/events.elf: c/events.c
	$(CLANG) -target bpf -g -O2 -c $< -o $@

rust: rust/fixture.elf

# We invoke cargo from inside the rust/ subdir so .cargo/config.toml
# is picked up. The DYLD_FALLBACK_LIBRARY_PATH export is only set on
# macOS; on Linux the bpf-linker proxy finds libLLVM automatically.
rust/fixture.elf: rust/Cargo.toml rust/src/main.rs rust/.cargo/config.toml
	cd rust && \
	  $(if $(HOMEBREW_LLVM_LIB),DYLD_FALLBACK_LIBRARY_PATH=$(HOMEBREW_LLVM_LIB)) \
	  $(CARGO) +nightly build --release
	cp rust/target/bpfel-unknown-none/release/fixture rust/fixture.elf

clean:
	rm -f c/events.elf rust/fixture.elf
	rm -rf rust/target
