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
  # does not understand `-target bpf`. Probe both Apple-silicon and
  # Intel Homebrew prefixes.
  CLANG := $(firstword \
    $(wildcard /opt/homebrew/opt/llvm/bin/clang) \
    $(wildcard /usr/local/opt/llvm/bin/clang) \
    $(CLANG))
  HOMEBREW_LLVM_LIB := $(firstword \
    $(wildcard /opt/homebrew/opt/llvm/lib) \
    $(wildcard /usr/local/opt/llvm/lib))
endif

CARGO ?= cargo
ZIG ?= zig

.PHONY: all c rust zig clean

all: c rust zig

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

zig: zig/events.o

# Zig 0.16+ supports `bpfel-linux-none` as a target. The `-fno-strip`
# flag is what causes the LLVM backend to emit the .BTF section.
# `-o events.o` is explicit so the rule keeps working if the source
# file is ever renamed.
zig/events.o: zig/events.zig
	cd zig && $(ZIG) build-obj -target bpfel-linux-none -ODebug -fno-strip -o events.o events.zig

clean:
	rm -f c/events.elf rust/fixture.elf zig/events.o
	rm -rf rust/target zig/.zig-cache
