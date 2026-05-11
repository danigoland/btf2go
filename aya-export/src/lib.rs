//! `btf2go-aya-export` — helper macro for btf2go's `--aya` flag.
//!
//! When `aya-ebpf`'s `HashMap<K, V>` (or `LruHashMap<K, V>`, `Array<V>`)
//! wraps a struct `V`, rustc lowers the wrapper's instantiation into BTF
//! but does not emit `V` itself as a standalone BTF entry — `V`'s bytes
//! live inside `PhantomData<(K, V)>`, which is a ZST.
//!
//! `btf2go generate --aya` resolves map value types via a four-tier
//! fallback lookup, but it requires `V` to already appear as a standalone
//! BTF entry in the ELF. This crate provides a one-liner that forces that:
//!
//! ```rust,ignore
//! use btf2go_aya_export::btf_export;
//!
//! #[repr(C)]
//! pub struct Foo { pub x: u64 }
//!
//! btf_export!(Foo);
//! // or multiple at once:
//! btf_export!(Foo, Bar, Baz);
//! ```
//!
//! Each invocation expands to a `#[no_mangle] static _BTF_EXPORT_<NAME>:
//! core::mem::MaybeUninit<T>` per type. The static's bytes are
//! uninitialized — only the type's presence in BTF matters. No `Default`
//! impl is required.

#![no_std]

/// Forces one or more types into the ELF's BTF section so `btf2go --aya`
/// can resolve them as map value types.
///
/// # Usage
///
/// Single type:
/// ```rust,ignore
/// btf_export!(Foo);
/// ```
///
/// Multiple types:
/// ```rust,ignore
/// btf_export!(Foo, Bar, Baz);
/// ```
///
/// Each call emits:
/// ```rust,ignore
/// #[no_mangle]
/// #[link_section = ".rodata"]
/// #[allow(non_upper_case_globals)]
/// pub static _BTF_EXPORT_FOO: core::mem::MaybeUninit<Foo> =
///     core::mem::MaybeUninit::uninit();
/// ```
///
/// `MaybeUninit::uninit()` is `const fn` (stable since Rust 1.36), so the
/// static requires no initialiser and no `Default` impl on the type.
#[macro_export]
macro_rules! btf_export {
    ($($T:ident),+ $(,)?) => {
        $(
            $crate::__btf_export_inner!($T);
        )+
    };
}

#[doc(hidden)]
#[macro_export]
macro_rules! __btf_export_inner {
    ($T:ident) => {
        $crate::__paste! {
            #[no_mangle]
            #[link_section = ".rodata"]
            #[allow(non_upper_case_globals)]
            pub static [<_BTF_EXPORT_ $T:upper>]: core::mem::MaybeUninit<$T> =
                core::mem::MaybeUninit::uninit();
        }
    };
}

#[doc(hidden)]
pub use paste::paste as __paste;
