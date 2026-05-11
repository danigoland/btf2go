#![no_std]
#![no_main]

use aya_ebpf::{
    macros::map,
    maps::{Array, HashMap, LruHashMap},
};
use btf2go_aya_export::btf_export;

#[repr(C)]
pub struct ScaffoldPing {
    pub timestamp_ns: u64,
    pub pid: u32,
    pub _pad: u32,
}

#[repr(C)]
pub struct BinaryIdentity {
    pub inode: u64,
    pub dev: u64,
}

#[repr(C)]
pub struct FooEvent {
    pub kind: u32,
    pub seq: u32,
    pub data: [u8; 16],
}

#[repr(C)]
pub struct BareStruct {
    pub a: u32,
    pub b: u32,
}

#[map]
static SCAFFOLD_LSM: HashMap<u64, ScaffoldPing> = HashMap::with_max_entries(1, 0);

#[map]
static BINARY_IDENTITIES: Array<BinaryIdentity> = Array::with_max_entries(64, 0);

#[map]
static FOOS: LruHashMap<u32, FooEvent> = LruHashMap::with_max_entries(128, 0);

// Force BareStruct into BTF via a function signature — exercises the
// existing non-aya path inside the same fixture.
#[no_mangle]
pub extern "C" fn use_bare(_b: &BareStruct) -> u32 {
    0
}

// Force the V types into BTF as standalone entries so --aya can find
// them via the terminal-segment fallback. Uses btf2go-aya-export macro
// instead of manual #[no_mangle] statics.
btf_export!(ScaffoldPing, BinaryIdentity, FooEvent);

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
