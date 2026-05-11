//! Compile-check that the macro expands cleanly.
//!
//! Not run as eBPF — the point is to assert the macro itself compiles on
//! stable Rust.

use btf2go_aya_export::btf_export;

#[repr(C)]
pub struct Foo {
    pub x: u64,
    pub y: u32,
}

#[repr(C)]
pub struct Bar {
    pub a: u8,
    pub b: u8,
    pub _pad: [u8; 6],
    pub c: u64,
}

// Multi-arg form exercises both single and multi-type paths in one call:
btf_export!(Foo, Bar);

fn main() {
    println!("compiled ok");
}
