#![no_std]
#![no_main]

use aya_ebpf::{macros::tracepoint, programs::TracePointContext};

// Mirror of the C fixture's events_t in Rust with namespace mangling
// from a sub-module (so the sanitizer in btfparser has work to do).

mod my_module {
    #[repr(C)]
    #[derive(Copy, Clone)]
    pub struct InnerT {
        pub lo: u32,
        pub hi: u32,
    }

    #[repr(C)]
    pub union PayloadT {
        pub raw: u64,
        pub pair: InnerT,
    }

    #[repr(C)]
    pub struct EventsT {
        pub kind: u8,
        // 3 bytes implicit pad
        pub pid: u32,
        // bitfields: not natively expressible in Rust; we emit a
        // single packed-byte field so the BTF would NOT have
        // bitfield members for it. That's fine — the goal here is
        // to prove btf2go handles a Rust-emitted BTF for the
        // common-case fields, not specifically to round-trip
        // bitfields (which Rust represents differently anyway).
        pub flags_and_prio: u8,
        // 7 bytes implicit pad
        pub ts: u64,
        pub comm: [u8; 16],
        pub pay: PayloadT,
    }

    // FlagsT exists to give the bool-detection path a CI-visible
    // regression target. rustc emits Rust `bool` as
    // `btf.Int{Size: 1, Name: "bool"}` with no special encoding flag,
    // so before v0.2.0 these would have rendered as `uint8` in the
    // generated Go.
    #[repr(C)]
    pub struct FlagsT {
        pub enabled: bool,
        pub readonly: bool,
        pub debug: bool,
        // 1 byte implicit pad
        pub seq: u32,
    }
}

// Anchor the type so BTF survives.
#[no_mangle]
#[link_section = ".rodata"]
pub static EVENTS_ANCHOR: my_module::EventsT = my_module::EventsT {
    kind: 0,
    pid: 0,
    flags_and_prio: 0,
    ts: 0,
    comm: [0; 16],
    pay: my_module::PayloadT { raw: 0 },
};

#[no_mangle]
#[link_section = ".rodata"]
pub static FLAGS_ANCHOR: my_module::FlagsT = my_module::FlagsT {
    enabled: false,
    readonly: false,
    debug: false,
    seq: 0,
};

#[tracepoint]
pub fn fixture(_ctx: TracePointContext) -> u32 {
    0
}

#[cfg(not(test))]
#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
