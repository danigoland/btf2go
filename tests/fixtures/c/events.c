// Minimal BTF fixture for btf2go E2E tests.
// Compiled with `clang -target bpf -g -O2 -c events.c -o events.elf`.
// We do not include any kernel headers — btf2go only needs the BTF
// type graph emitted by `-g`, not actual eBPF program semantics.

typedef unsigned char  u8;
typedef unsigned short u16;
typedef unsigned int   u32;
typedef unsigned long long u64;

struct inner_t {
    u32 lo;
    u32 hi;
};

union payload_t {
    u64 raw;
    struct inner_t pair;
};

struct events_t {
    u8  kind;             // offset 0
    // 3 bytes implicit pad
    u32 pid;              // offset 4
    u8  flag_a : 1;       // bitfield run at byte 8
    u8  flag_b : 1;
    u8  prio   : 6;
    // 7 bytes implicit pad to 8-byte align ts
    u64 ts;               // offset 16
    char comm[16];        // offset 24
    union payload_t pay;  // offset 40
};

// Anchor the struct so it survives into the emitted BTF.
struct events_t __events_anchor;
