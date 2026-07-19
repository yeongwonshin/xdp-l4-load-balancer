# BPF Maps

## services

Type: `bpf_map_type_hash`

Maximum entries: 4096

Key fields:

- `vip` as IPv4 network-order bytes
- `port` as TCP or UDP destination port network-order bytes
- `proto` as 6 for TCP or 17 for UDP

Value fields:

- `backend_start` as the first backend identifier for the service
- `backend_count` as the size of the contiguous backend range
- `flags` with the DSR flag

The control plane rejects duplicate service keys. The datapath validates that the complete backend range fits within the backend array before selecting an entry.

## backends

Type: `bpf_map_type_array`

Maximum entries: 65536

Value fields:

- backend IPv4 address for labels and future forwarding modes
- egress interface index
- backend destination MAC
- egress interface source MAC

The current DSR datapath uses the interface index and both MAC addresses. It does not use the backend IP to rewrite packets.

## backend_stats

Type: `bpf_map_type_percpu_hash`

Maximum entries: 65536

Counters per backend and CPU:

- packets selected for redirect
- bytes selected for redirect
- newly inserted flows

The userspace collector aggregates all possible CPU values for each configured backend.

## flow_table

Type: `bpf_map_type_lru_hash`

Maximum entries: 1048576

Key fields:

- source IPv4 address
- destination IPv4 address
- source port
- destination port
- protocol

Value fields:

- selected backend identifier
- last seen monotonic timestamp in nanoseconds

The timestamp is updated for existing flows but is not currently used for explicit expiration. LRU pressure can evict old entries.

## datapath_stats

Type: `bpf_map_type_percpu_array`

Keys represent fixed datapath events:

- `packets_seen`
- `non_ipv4`
- `malformed`
- `fragmented`
- `unsupported_l4`
- `service_miss`
- `flow_insert_failure`
- `backend_miss`
- `redirect_requested`

The Prometheus collector exports these values through `xdp_l4lb_datapath_events_total` with an `event` label.
