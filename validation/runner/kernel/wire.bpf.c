// T2.5 fixture for btf2go validation. Deliberately exercises every
// alignment edge case the unit tests cover: bitfield run, packed
// uint64, nested union, char array, mixed signed/unsigned ints.

#include <linux/bpf.h>
#include <bpf/bpf_helpers.h>

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

struct wire_t {
    u8  kind;
    // 3 bytes pad
    u32 pid;
    u8  flag_a : 1;
    u8  flag_b : 1;
    u8  prio   : 6;
    // 7 bytes pad
    u64 ts;
    char comm[16];
    union payload_t pay;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __type(key, u32);
    __type(value, struct wire_t);
    __uint(max_entries, 16);
} wire_map SEC(".maps");

SEC("tracepoint/syscalls/sys_enter_execve")
int handle_execve(void *ctx) { return 0; }

char _license[] SEC("license") = "GPL";
