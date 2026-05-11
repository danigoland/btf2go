#![no_std]
#![no_main]

use aya_ebpf::{macros::map, maps::HashMap};

#[repr(C)]
pub struct ScaffoldPing {
    pub timestamp_ns: u64,
    pub pid: u32,
    pub _pad: u32,
}

// No _BTF_EXPORT_* static — ScaffoldPing is referenced ONLY via the
// HashMap wrapper's PhantomData. rustc does not emit a standalone
// BTF entry for it.
#[map]
static SCAFFOLD_LSM: HashMap<u64, ScaffoldPing> = HashMap::with_max_entries(1, 0);

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
