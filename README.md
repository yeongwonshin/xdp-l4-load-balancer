# eBPF/XDP L4 Load Balancer

A portfolio-oriented XDP-based Layer 4 load balancer scaffold. The XDP hook parses TCP and UDP packets, looks up a service by `VIP:Port:Protocol`, and selects a backend using a 5-tuple hash. The selected backend is stored in a flow table so that packets belonging to the same connection continue to use the same backend.

> The current implementation uses DSR/L2 redirect mode. The VIP destination IP remains unchanged, while only the Ethernet destination MAC is rewritten to the backend MAC before the packet is forwarded to the egress interface with `bpf_redirect()`. Each backend must configure the VIP on a loopback or dummy interface.

## Key Features

- Parse Ethernet, IPv4, TCP, and UDP packets in an XDP program
- Look up services by `VIP + Port + Protocol`
- Select backends using a 5-tuple hash
- Maintain connection affinity with an LRU flow table
- Rewrite and redirect TCP/UDP packets at Layer 2
- Track packet, byte, and flow counters per backend
- Provide a Go-based userspace control plane
- Export Prometheus metrics through `/metrics`
- Include a Grafana dashboard scaffold

## Architecture

```text
client packet
    |
    v
NIC RX -> XDP hook
    |
    +-- parse Ethernet / IPv4 / TCP|UDP
    +-- services map: VIP:Port:Proto -> backend range
    +-- flow_table map: 5-tuple -> backend_id
    +-- backends map: backend_id -> MAC, ifindex
    +-- backend_stats map: backend_id -> packets/bytes/flows
    |
    v
rewrite L2 dst/src MAC -> bpf_redirect(ifindex)
```

## Directory Structure

```text
xdp-l4-lb/
├── bpf/                         # eBPF/XDP datapath
│   ├── common.h                 # Shared map and value structure definitions
│   └── xdp_l4lb.bpf.c           # XDP packet parser, hashing, and redirect logic
├── cmd/xdp-l4-lb/               # Go control plane
│   ├── main.go                  # Load and attach XDP program; run metrics server
│   ├── config.go                # YAML configuration parser
│   ├── maps.go                  # Service and backend map programming
│   ├── metrics.go               # Prometheus collector
│   └── types.go                 # Go-side map structures
├── configs/
│   └── example.yaml             # Sample VIP and backend configuration
├── deploy/
│   ├── docker-compose.yml       # Prometheus and Grafana
│   ├── prometheus.yml
│   └── grafana/dashboards/
│       └── xdp-l4-lb.json
├── docs/
│   ├── architecture.md
│   ├── lab-topology.md
│   ├── maps.md
│   └── test-plan.md
├── scripts/
│   ├── run-dev.sh
│   └── setup-dsr-backend.sh
├── testdata/
│   └── sample-metrics.txt
├── Makefile
├── go.mod
└── README.md
```

## Build

Required tools:

- Linux kernel with eBPF/XDP support
- clang/llvm
- bpftool or kernel headers
- Go 1.22+
- Root privileges or the required capabilities

```bash
make build
```

`go generate` runs `bpf2go` to generate the eBPF object and Go loader bindings.

## Run

```bash
sudo ./bin/xdp-l4-lb \
  -iface eth0 \
  -config configs/example.yaml \
  -xdp-mode generic \
  -metrics :2112
```

For production or performance testing, consider `-xdp-mode driver` first. If the NIC driver does not support native XDP, start with `generic` mode.

## Metrics

```bash
curl localhost:2112/metrics
```

Exposed metrics:

- `xdp_l4lb_backend_packets_total`
- `xdp_l4lb_backend_bytes_total`
- `xdp_l4lb_backend_flows_total`

Prometheus and Grafana:

```bash
cd deploy
docker compose up -d
```

## DSR Backend Configuration Example

Configure the VIP on the backend server's loopback interface and suppress ARP responses.

```bash
sudo ./scripts/setup-dsr-backend.sh 10.10.0.100
```

## Portfolio Extension Ideas

1. Add NAT mode with destination IP rewriting and checksum updates
2. Add consistent hashing or Maglev hashing
3. Add a health checker and dynamic backend removal
4. Add a control-plane REST API
5. Add zero-downtime reloads using pinned maps
6. Add ACL and SYN flood mitigation with `XDP_DROP`
7. Integrate a TC egress hook for SNAT and return-path handling
