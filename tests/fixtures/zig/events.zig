// Minimal BTF fixture for btf2go E2E tests, Zig flavour.
// Compiled with `zig build-obj -target bpfel-linux-none -ODebug events.zig`
// (or via `make -C tests/fixtures zig`).
//
// Mirrors the C fixture intentionally so users can compare layouts
// across toolchains. Zig is `extern struct` for repr(C) and
// `packed union` for unions; the BTF should look the same as the
// clang-emitted version.

const InnerT = extern struct {
    lo: u32,
    hi: u32,
};

const PayloadT = extern union {
    raw: u64,
    pair: InnerT,
};

const EventsT = extern struct {
    kind: u8,
    // 3 bytes implicit pad
    pid: u32,
    // We don't try to express C bitfields in Zig — Zig has its own
    // packed-struct semantics. A single u8 stand-in keeps the
    // layout matching the Rust fixture.
    flags_and_prio: u8,
    // 7 bytes implicit pad to align ts
    ts: u64,
    comm: [16]u8,
    pay: PayloadT,
};

// Anchor the type so BTF survives. `export` puts the symbol in a
// real section (not stripped) and `extern` ensures it doesn't get
// const-folded away.
export const events_anchor: EventsT = EventsT{
    .kind = 0,
    .pid = 0,
    .flags_and_prio = 0,
    .ts = 0,
    .comm = [_]u8{0} ** 16,
    .pay = PayloadT{ .raw = 0 },
};
