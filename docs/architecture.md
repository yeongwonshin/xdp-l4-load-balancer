# Architecture

## Scope

The project provides an IPv4 TCP and UDP load balancer using an XDP ingress program and a Go control plane. Forwarding uses direct server return at Layer 2. The datapath does not rewrite IP addresses or transport checksums.

## Datapath sequence

1. Count the received frame.
2. Validate the Ethernet header.
3. Parse up to two 802.1q or 802.1ad VLAN headers.
4. Pass non-IPv4 traffic to the host stack.
5. Validate the IPv4 version, header length, total length, and frame bounds.
6. Pass IPv4 fragments to the host stack because non-initial fragments do not contain the complete transport header.
7. Parse TCP or UDP source and destination ports.
8. Look up the service by VIP, destination port, and protocol.
9. Look up the five tuple in the LRU flow table.
10. For a new flow, hash the tuple and insert the selected backend with `bpf_noexist`.
11. Resolve the backend value and update per-cpu counters.
12. Rewrite Ethernet destination and source addresses.
13. Return `bpf_redirect` for the backend egress interface.

All parser failures and policy outcomes are represented in `datapath_stats`. The program chooses fail-open behavior and returns `xdp_pass` for traffic it cannot safely process.

## Byte order contract

Service keys use the exact byte representation found in the packet headers. The Go control plane converts IPv4 addresses and ports so that their in-memory representation matches network-order bytes in BPF map keys. This is important on little-endian systems because storing the human-readable numeric IPv4 value directly would reverse the key bytes and cause service misses.

## Flow affinity

The flow key contains source and destination IPv4 addresses, source and destination ports, and protocol. New flows use a verifier-friendly FNV-style hash and modulo selection within the contiguous backend range assigned to a service.

The control plane rejects duplicate service keys. The datapath checks backend range bounds before calculating the backend identifier. First-flow insertion uses `bpf_noexist`; if another CPU wins the insertion race, the program reuses the stored backend instead of overwriting it.

The flow map is an LRU hash. It has no explicit idle timeout. Entries can be evicted when the map is under pressure, after which a later packet can be assigned again.

## Layer 2 direct server return

Each backend map entry stores both destination and source Ethernet addresses:

- destination MAC is configured for the backend
- source MAC is read from the selected egress interface

This avoids assuming that the XDP ingress interface and every backend egress interface share the same MAC address. The VIP remains unchanged in the IPv4 header. The backend accepts the VIP locally and sends the response directly to the client according to its routing configuration.

## Per-cpu accounting

Backend counters and datapath outcome counters use per-cpu maps. Each CPU updates its own value without a shared atomic operation on the hot path. The Prometheus collector reads all possible CPU slots and sums them before exposition.

The backend counters are:

- packets selected for redirect
- bytes selected for redirect
- newly inserted flows

The datapath event counters include parser outcomes, service misses, flow insertion failures, backend misses, and redirect requests.

## Control plane lifecycle

The Go process performs the following sequence:

1. Strictly decode and normalize YAML.
2. Validate service uniqueness, backend limits, MAC addresses, protocols, and modes.
3. Resolve backend egress interfaces and their source MAC addresses.
4. Remove the memlock limit.
5. Load the generated eBPF collection.
6. Program backend and service maps.
7. Attach XDP to the selected interface.
8. Start Prometheus and health endpoints.
9. Wait for a termination signal or HTTP server failure.
10. Gracefully stop HTTP serving, detach XDP, and close BPF objects.

## Failure model

The datapath is fail-open for malformed, unsupported, fragmented, or unconfigured traffic. A valid configured packet is also passed when its backend entry is unavailable. This behavior protects host connectivity during configuration errors but is not an access-control policy.

`bpf_redirect` returns an XDP redirect action request. Driver, device, or map-level transmit errors can occur after the program returns. Monitor XDP redirect error tracepoints, interface drop counters, and backend traffic in addition to application metrics.
