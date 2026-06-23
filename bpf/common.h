#ifndef __XDP_L4LB_COMMON_H
#define __XDP_L4LB_COMMON_H

#include <linux/types.h>

#define MAX_SERVICES 4096
#define MAX_BACKENDS 65536
#define MAX_FLOWS 1048576

#define L4_PROTO_TCP 6
#define L4_PROTO_UDP 17

#define LB_FLAG_DSR 0x1
#define LB_FLAG_SNAT 0x2 /* reserved for future NAT mode */

struct service_key {
    __u32 vip;      /* network byte order */
    __u16 port;     /* network byte order */
    __u8 proto;     /* TCP=6, UDP=17 */
    __u8 pad;
};

struct service_value {
    __u32 backend_start;
    __u32 backend_count;
    __u32 flags;
    __u32 reserved;
};

struct backend_value {
    __u32 ip;       /* network byte order, kept for metrics / future NAT */
    __u32 ifindex;  /* egress interface index */
    __u8 mac[6];    /* backend destination MAC */
    __u8 pad[2];
};

struct backend_stats {
    __u64 packets;
    __u64 bytes;
    __u64 flows;
};

struct flow_key {
    __u32 src_ip;
    __u32 dst_ip;
    __u16 src_port;
    __u16 dst_port;
    __u8 proto;
    __u8 pad[3];
};

struct flow_value {
    __u32 backend_id;
    __u32 pad;
    __u64 last_seen_ns;
};

struct lb_config {
    __u8 src_mac[6];
    __u8 pad[2];
    __u32 default_action; /* XDP_PASS by default */
};

#endif /* __XDP_L4LB_COMMON_H */
