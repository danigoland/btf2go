#![no_std]
#![no_main]

use aya_ebpf::{
    macros::map,
    maps::{Array, HashMap, LruHashMap},
};

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
// them via the terminal-segment fallback. The values are zero-init
// placeholders — only their type matters.
#[no_mangle]
static _BTF_EXPORT_SCAFFOLD_PING: ScaffoldPing = ScaffoldPing {
    timestamp_ns: 0,
    pid: 0,
    _pad: 0,
};
#[no_mangle]
static _BTF_EXPORT_BINARY_IDENTITY: BinaryIdentity = BinaryIdentity { inode: 0, dev: 0 };
#[no_mangle]
static _BTF_EXPORT_FOO_EVENT: FooEvent = FooEvent {
    kind: 0,
    seq: 0,
    data: [0; 16],
};

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
