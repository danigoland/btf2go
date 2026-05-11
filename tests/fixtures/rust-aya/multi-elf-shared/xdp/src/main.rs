#![no_std]
#![no_main]

use aya_ebpf::{macros::map, maps::HashMap};
use common::BinaryIdentity;

#[map]
static XDP_IDS: HashMap<u64, BinaryIdentity> = HashMap::with_max_entries(64, 0);

// Force BinaryIdentity into BTF as a standalone entry so the --aya
// bridge can resolve it.
#[no_mangle]
static _BTF_EXPORT_BINARY_IDENTITY: BinaryIdentity = BinaryIdentity { inode: 0, dev: 0 };

#[panic_handler]
fn panic(_info: &core::panic::PanicInfo) -> ! {
    loop {}
}
