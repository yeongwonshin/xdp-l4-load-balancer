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
    __uint(type, BPF_MAP_TYPE_ARRAY);
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
    __uint(type, BPF_MAP_TYPE_ARRAY);
    __uint(max_entries, 1);
    __type(key, __u32);
    __type(value, struct lb_config);
} lb_config SEC(".maps");

static __always_inline int parse_l4_ports(void *data, void *data_end, struct iphdr *iph,
                                          __u16 *src_port, __u16 *dst_port)
{
    __u32 ip_header_len = iph->ihl * 4;
    void *l4 = (void *)iph + ip_header_len;

    if (l4 > data_end)
        return -1;

    if (iph->protocol == IPPROTO_TCP) {
        struct tcphdr *tcp = l4;
        if ((void *)(tcp + 1) > data_end)
            return -1;
        *src_port = tcp->source;
        *dst_port = tcp->dest;
        return 0;
    }

    if (iph->protocol == IPPROTO_UDP) {
        struct udphdr *udp = l4;
        if ((void *)(udp + 1) > data_end)
            return -1;
        *src_port = udp->source;
        *dst_port = udp->dest;
        return 0;
    }

    return -1;
}

static __always_inline __u32 hash_5tuple(struct flow_key *key)
{
    /* verifier-friendly FNV-1a style hash */
    __u32 h = 2166136261u;
    h = (h ^ key->src_ip) * 16777619u;
    h = (h ^ key->dst_ip) * 16777619u;
    h = (h ^ ((__u32)key->src_port << 16 | key->dst_port)) * 16777619u;
    h = (h ^ key->proto) * 16777619u;
    return h;
}

static __always_inline void rewrite_l2(struct ethhdr *eth, struct backend_value *backend,
                                       struct lb_config *cfg)
{
#pragma clang loop unroll(full)
    for (int i = 0; i < ETH_ALEN; i++) {
        eth->h_dest[i] = backend->mac[i];
        eth->h_source[i] = cfg->src_mac[i];
    }
}

SEC("xdp")
int xdp_l4lb(struct xdp_md *ctx)
{
    void *data = (void *)(long)ctx->data;
    void *data_end = (void *)(long)ctx->data_end;

    struct ethhdr *eth = data;
    if ((void *)(eth + 1) > data_end)
        return XDP_PASS;

    if (eth->h_proto != bpf_htons(ETH_P_IP))
        return XDP_PASS;

    struct iphdr *iph = (void *)(eth + 1);
    if ((void *)(iph + 1) > data_end)
        return XDP_PASS;

    if (iph->version != 4 || iph->ihl < 5)
        return XDP_PASS;

    __u16 src_port = 0;
    __u16 dst_port = 0;
    if (parse_l4_ports(data, data_end, iph, &src_port, &dst_port) < 0)
        return XDP_PASS;

    struct service_key svc_key = {
        .vip = iph->daddr,
        .port = dst_port,
        .proto = iph->protocol,
    };

    struct service_value *svc = bpf_map_lookup_elem(&services, &svc_key);
    if (!svc || svc->backend_count == 0)
        return XDP_PASS;

    struct flow_key fkey = {
        .src_ip = iph->saddr,
        .dst_ip = iph->daddr,
        .src_port = src_port,
        .dst_port = dst_port,
        .proto = iph->protocol,
    };

    __u32 backend_id;
    struct flow_value *existing = bpf_map_lookup_elem(&flow_table, &fkey);
    if (existing) {
        backend_id = existing->backend_id;
        existing->last_seen_ns = bpf_ktime_get_ns();
    } else {
        __u32 slot = hash_5tuple(&fkey) % svc->backend_count;
        backend_id = svc->backend_start + slot;

        struct flow_value fval = {
            .backend_id = backend_id,
            .last_seen_ns = bpf_ktime_get_ns(),
        };
        bpf_map_update_elem(&flow_table, &fkey, &fval, BPF_ANY);

        struct backend_stats *new_flow_stats = bpf_map_lookup_elem(&backend_stats, &backend_id);
        if (new_flow_stats)
            __sync_fetch_and_add(&new_flow_stats->flows, 1);
    }

    struct backend_value *backend = bpf_map_lookup_elem(&backends, &backend_id);
    if (!backend || backend->ifindex == 0)
        return XDP_PASS;

    __u32 cfg_key = 0;
    struct lb_config *cfg = bpf_map_lookup_elem(&lb_config, &cfg_key);
    if (!cfg)
        return XDP_PASS;

    struct backend_stats *stats = bpf_map_lookup_elem(&backend_stats, &backend_id);
    if (stats) {
        __u64 bytes = data_end - data;
        __sync_fetch_and_add(&stats->packets, 1);
        __sync_fetch_and_add(&stats->bytes, bytes);
    }

    /* DSR/L2 mode: keep VIP as destination IP, rewrite only Ethernet header. */
    rewrite_l2(eth, backend, cfg);
    return bpf_redirect(backend->ifindex, 0);
}
