#ifndef __XDP_L4LB_COMMON_H
#define __XDP_L4LB_COMMON_H

#include <linux/types.h>

#define MAX_SERVICES 4096
#define MAX_BACKENDS 65536
#define MAX_FLOWS 1048576

#define L4_PROTO_TCP 6
#define L4_PROTO_UDP 17

#define LB_FLAG_DSR 0x1

#define IPV4_FLAG_MORE_FRAGMENTS 0x2000
#define IPV4_FRAGMENT_OFFSET_MASK 0x1fff

enum datapath_event {
    DATAPATH_EVENT_PACKETS_SEEN = 0,
    DATAPATH_EVENT_NON_IPV4,
    DATAPATH_EVENT_MALFORMED,
    DATAPATH_EVENT_FRAGMENTED,
    DATAPATH_EVENT_UNSUPPORTED_L4,
    DATAPATH_EVENT_SERVICE_MISS,
    DATAPATH_EVENT_FLOW_INSERT_FAILURE,
    DATAPATH_EVENT_BACKEND_MISS,
    DATAPATH_EVENT_REDIRECT_REQUESTED,
    DATAPATH_EVENT_MAX,
};

struct service_key {
    __u32 vip;      /* network byte order in map memory */
    __u16 port;     /* network byte order in map memory */
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
    __u32 ip;       /* network byte order in map memory */
    __u32 ifindex;  /* egress interface index */
    __u8 dst_mac[6];
    __u8 src_mac[6];
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

#endif /* __XDP_L4LB_COMMON_H */
