// SPDX-License-Identifier: GPL-2.0 OR BSD-3-Clause
#include <linux/bpf.h>
#include <linux/if_ether.h>
#include <linux/ip.h>
#include <linux/in.h>
#include <linux/tcp.h>
#include <linux/udp.h>
#include <bpf/bpf_helpers.h>
#include <bpf/bpf_endian.h>
#include "common.h"

char LICENSE[] SEC("license") = "Dual BSD/GPL";

struct vlan_hdr {
    __be16 tci;
    __be16 encapsulated_proto;
};

struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, MAX_SERVICES);
    __type(key, struct service_key);
    __type(value, struct service_value);
} services SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, MAX_BACKENDS);
    __type(key, __u32);
    __type(value, struct backend_value);
} backends SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_HASH);
    __uint(max_entries, MAX_BACKENDS);
    __type(key, __u32);
    __type(value, struct backend_stats);
} backend_stats SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_LRU_HASH);
    __uint(max_entries, MAX_FLOWS);
    __type(key, struct flow_key);
    __type(value, struct flow_value);
} flow_table SEC(".maps");

struct {
    __uint(type, BPF_MAP_TYPE_PERCPU_ARRAY);
    __uint(max_entries, DATAPATH_EVENT_MAX);
    __type(key, __u32);
    __type(value, __u64);
} datapath_stats SEC(".maps");

static __always_inline void record_event(__u32 event)
{
    if (event >= DATAPATH_EVENT_MAX)
        return;

    __u64 *counter = bpf_map_lookup_elem(&datapath_stats, &event);
    if (counter)
        (*counter)++;
}

static __always_inline int parse_eth_protocol(void *data, void *data_end,
                                               struct ethhdr **eth_out,
                                               __be16 *protocol_out,
                                               __u64 *network_offset_out)
{
    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return -1;

    __be16 protocol = eth->h_proto;
    __u64 offset = sizeof(*eth);

#pragma clang loop unroll(full)
    for (int i = 0; i < 2; i++) {
        if (protocol != bpf_htons(ETH_P_8021Q) &&
            protocol != bpf_htons(ETH_P_8021AD))
            break;

        struct vlan_hdr *vlan = data + offset;
        if ((void *)(vlan + 1) > data_end)
            return -1;

        protocol = vlan->encapsulated_proto;
        offset += sizeof(*vlan);
    }

    *eth_out = eth;
    *protocol_out = protocol;
    *network_offset_out = offset;
    return 0;
}

static __always_inline int parse_l4_ports(struct iphdr *iph, void *ip_end,
                                          __u16 *src_port, __u16 *dst_port)
{
    __u32 ip_header_len = (__u32)iph->ihl * 4;
    void *l4 = (void *)iph + ip_header_len;

    if (l4 > ip_end)
        return -1;

    if (iph->protocol == IPPROTO_TCP) {
        struct tcphdr *tcp = l4;
        if ((void *)(tcp + 1) > ip_end || tcp->doff < 5)
            return -1;

        __u32 tcp_header_len = (__u32)tcp->doff * 4;
        if ((void *)tcp + tcp_header_len > ip_end)
            return -1;

        *src_port = tcp->source;
        *dst_port = tcp->dest;
        return 0;
    }

    if (iph->protocol == IPPROTO_UDP) {
        struct udphdr *udp = l4;
        if ((void *)(udp + 1) > ip_end)
            return -1;

        __u16 udp_len = bpf_ntohs(udp->len);
        if (udp_len < sizeof(*udp) || (void *)udp + udp_len > ip_end)
            return -1;

        *src_port = udp->source;
        *dst_port = udp->dest;
        return 0;
    }

    return 1;
}

static __always_inline __u32 hash_5tuple(const struct flow_key *key)
{
    __u32 h = 2166136261u;
    h = (h ^ key->src_ip) * 16777619u;
    h = (h ^ key->dst_ip) * 16777619u;
    h = (h ^ ((__u32)key->src_port << 16 | key->dst_port)) * 16777619u;
    h = (h ^ key->proto) * 16777619u;
    return h;
}

static __always_inline void rewrite_l2(struct ethhdr *eth,
                                       const struct backend_value *backend)
{
#pragma clang loop unroll(full)
    for (int i = 0; i < ETH_ALEN; i++) {
        eth->h_dest[i] = backend->dst_mac[i];
        eth->h_source[i] = backend->src_mac[i];
    }
}

SEC("xdp")
int xdp_l4lb(struct xdp_md *ctx)
{
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;

    record_event(DATAPATH_EVENT_PACKETS_SEEN);

    struct ethhdr *eth = 0;
    __be16 protocol = 0;
    __u64 network_offset = 0;
    if (parse_eth_protocol(data, data_end, &eth, &protocol, &network_offset) < 0) {
        record_event(DATAPATH_EVENT_MALFORMED);
        return XDP_PASS;
    }

    if (protocol != bpf_htons(ETH_P_IP)) {
        record_event(DATAPATH_EVENT_NON_IPV4);
        return XDP_PASS;
    }

    struct iphdr *iph = data + network_offset;
    if ((void *)(iph + 1) > data_end) {
        record_event(DATAPATH_EVENT_MALFORMED);
        return XDP_PASS;
    }

    if (iph->version != 4 || iph->ihl < 5) {
        record_event(DATAPATH_EVENT_MALFORMED);
        return XDP_PASS;
    }

    __u32 ip_header_len = (__u32)iph->ihl * 4;
    if ((void *)iph + ip_header_len > data_end) {
        record_event(DATAPATH_EVENT_MALFORMED);
        return XDP_PASS;
    }

    __u16 total_len = bpf_ntohs(iph->tot_len);
    if (total_len < ip_header_len || (void *)iph + total_len > data_end) {
        record_event(DATAPATH_EVENT_MALFORMED);
        return XDP_PASS;
    }

    __u16 fragment = bpf_ntohs(iph->frag_off);
    if (fragment & (IPV4_FLAG_MORE_FRAGMENTS | IPV4_FRAGMENT_OFFSET_MASK)) {
        record_event(DATAPATH_EVENT_FRAGMENTED);
        return XDP_PASS;
    }

    __u16 src_port = 0;
    __u16 dst_port = 0;
    int l4_result = parse_l4_ports(iph, (void *)iph + total_len, &src_port, &dst_port);
    if (l4_result > 0) {
        record_event(DATAPATH_EVENT_UNSUPPORTED_L4);
        return XDP_PASS;
    }
    if (l4_result < 0) {
        record_event(DATAPATH_EVENT_MALFORMED);
        return XDP_PASS;
    }

    struct service_key svc_key = {
        .vip = iph->daddr,
        .port = dst_port,
        .proto = iph->protocol,
    };

    struct service_value *svc = bpf_map_lookup_elem(&services, &svc_key);
    if (!svc || svc->backend_count == 0 ||
        svc->backend_start >= MAX_BACKENDS ||
        svc->backend_count > MAX_BACKENDS - svc->backend_start) {
        record_event(DATAPATH_EVENT_SERVICE_MISS);
        return XDP_PASS;
    }

    struct flow_key fkey = {
        .src_ip = iph->saddr,
        .dst_ip = iph->daddr,
        .src_port = src_port,
        .dst_port = dst_port,
        .proto = iph->protocol,
    };

    __u64 now = bpf_ktime_get_ns();
    __u32 backend_id = 0;
    struct flow_value *existing = bpf_map_lookup_elem(&flow_table, &fkey);
    if (existing) {
        backend_id = existing->backend_id;
        existing->last_seen_ns = now;
    } else {
        __u32 slot = hash_5tuple(&fkey) % svc->backend_count;
        backend_id = svc->backend_start + slot;

        struct flow_value fval = {
            .backend_id = backend_id,
            .last_seen_ns = now,
        };

        int insert_result = bpf_map_update_elem(&flow_table, &fkey, &fval, BPF_NOEXIST);
        if (insert_result == 0) {
            struct backend_stats *new_flow_stats =
                bpf_map_lookup_elem(&backend_stats, &backend_id);
            if (new_flow_stats)
                new_flow_stats->flows++;
        } else {
            existing = bpf_map_lookup_elem(&flow_table, &fkey);
            if (existing) {
                backend_id = existing->backend_id;
                existing->last_seen_ns = now;
            } else {
                record_event(DATAPATH_EVENT_FLOW_INSERT_FAILURE);
            }
        }
    }

    if (backend_id >= MAX_BACKENDS) {
        record_event(DATAPATH_EVENT_BACKEND_MISS);
        return XDP_PASS;
    }

    struct backend_value *backend = bpf_map_lookup_elem(&backends, &backend_id);
    if (!backend || backend->ifindex == 0) {
        record_event(DATAPATH_EVENT_BACKEND_MISS);
        return XDP_PASS;
    }

    struct backend_stats *stats = bpf_map_lookup_elem(&backend_stats, &backend_id);
    if (stats) {
        stats->packets++;
        stats->bytes += (__u64)(data_end - data);
    }

    rewrite_l2(eth, backend);
    record_event(DATAPATH_EVENT_REDIRECT_REQUESTED);
    return bpf_redirect(backend->ifindex, 0);
}
