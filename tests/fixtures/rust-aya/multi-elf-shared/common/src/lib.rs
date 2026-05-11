#![no_std]

#[repr(C)]
pub struct BinaryIdentity {
    pub inode: u64,
    pub dev: u64,
}
