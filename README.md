# eBPF/XDP L4 Load Balancer

An IPv4 Layer 4 load balancer that makes TCP and UDP forwarding decisions in XDP before packets enter the host network stack. The userspace control plane validates a YAML configuration, loads the eBPF objects, programs service and backend maps, attaches the XDP program, and exposes Prometheus metrics and health endpoints.

The datapath implements direct server return at Layer 2. It keeps the VIP as the destination IP, rewrites the Ethernet destination MAC to the selected backend, rewrites the Ethernet source MAC to the selected egress interface MAC, and redirects the frame through that interface. Each backend must own the VIP on a loopback or dummy interface and must not answer ARP for it on the service network.

Detailed design, map layouts, deployment guidance, troubleshooting, and test procedures are maintained separately:

- [Architecture](docs/architecture.md)
- [BPF maps](docs/maps.md)
- [Operations and troubleshooting](docs/operations.md)
- [Lab topology](docs/lab-topology.md)
- [Test plan](docs/test-plan.md)

## What is included

- Ethernet and up to two VLAN header parsing
- IPv4 TCP and UDP service lookup by VIP, port, and protocol
- Five tuple flow affinity using an LRU hash map
- Race-safe first-flow insertion with deterministic backend selection
- Per-cpu backend and datapath counters
- Strict YAML decoding that rejects unknown fields, duplicate services, invalid MAC addresses, and map limit overflow
- Egress interface validation with per-backend source MAC selection
- Generic, driver, and hardware XDP attach modes
- Prometheus metrics, process metrics, `/healthz`, and `/readyz`
- Provisioned Prometheus and Grafana development stack
- Unit tests for configuration validation, byte order, map keys, and attach mode parsing

## Packet path

```text
client frame
    |
    v
xdp ingress
    |
    +-- ethernet and vlan validation
    +-- ipv4 header and length validation
    +-- fragment policy: pass to host stack
    +-- tcp or udp port parsing
    +-- services map lookup
    +-- flow table affinity lookup or insertion
    +-- backend map lookup
    +-- per-cpu counters
    +-- destination and source mac rewrite
    |
    v
bpf_redirect to backend egress interface
```

Packets that are malformed, fragmented, unsupported, or not addressed to a configured service are passed to the host stack. Datapath outcome counters make those decisions observable.

## Repository layout

```text
.
├── bpf/
│   ├── common.h
│   └── xdp_l4lb.bpf.c
├── cmd/xdp-l4lb/
│   ├── config.go
│   ├── main.go
│   ├── maps.go
│   ├── metrics.go
│   ├── types.go
│   └── *_test.go
├── configs/
│   └── example.yaml
├── deploy/
│   ├── docker-compose.yml
│   ├── prometheus.yml
│   └── grafana/
├── docs/
│   ├── architecture.md
│   ├── lab-topology.md
│   ├── maps.md
│   ├── operations.md
│   └── test-plan.md
├── scripts/
│   ├── generate-bpf.sh
│   ├── run-dev.sh
│   └── setup-dsr-backend.sh
├── Makefile
├── go.mod
└── README.md
```

## Requirements

- Linux with eBPF and XDP support
- clang with the BPF target
- libbpf development headers, including `bpf_helpers.h`
- Linux UAPI and architecture headers
- Go 1.22 or newer
- Root privileges or equivalent capabilities for loading and attaching eBPF programs
- An Ethernet egress interface for every configured backend

The BPF generation script selects the target architecture from `go env GOARCH` and supports amd64 and arm64.

## Build and validate

```bash
make build
make test
make vet
make check
```

Validate YAML without loading eBPF or requiring an interface:

```bash
./bin/xdp-l4lb -config configs/example.yaml -check-config
```

The configuration decoder is strict. Misspelled or unknown YAML fields fail validation instead of being silently ignored.

## Run

```bash
sudo ./bin/xdp-l4lb \
  -iface eth0 \
  -config configs/example.yaml \
  -xdp-mode generic \
  -metrics :2112
```

Start with generic mode when testing portability. Use driver mode when the NIC driver supports native XDP. Hardware offload requires compatible NIC and driver support.

The program detaches XDP and shuts down the HTTP server on `sigint` or `sigterm`.

## Configuration

```yaml
services:
  - name: web-vip
    vip: 10.10.0.100
    port: 80
    protocol: tcp
    mode: dsr
    backends:
      - name: web-1
        ip: 10.10.0.11
        mac: "02:42:0a:0a:00:0b"
        ifname: eth1
      - name: web-2
        ip: 10.10.0.12
        mac: "02:42:0a:0a:00:0c"
        ifindex: 4
```

`ifname` or `ifindex` is required for every backend. When both are provided, they must identify the same interface. The interface MAC is stored with the backend and used as the Ethernet source address during redirect. Backend IP is retained for labels and future routing modes; DSR forwarding uses the configured backend MAC and egress interface.

## Backend preparation

Run on each backend that serves the VIP:

```bash
sudo ./scripts/setup-dsr-backend.sh 10.10.0.100
```

An optional second argument selects the local device that receives the VIP:

```bash
sudo ./scripts/setup-dsr-backend.sh 10.10.0.100 dummy0
```

## Metrics and health

```bash
curl http://127.0.0.1:2112/healthz
curl http://127.0.0.1:2112/readyz
curl http://127.0.0.1:2112/metrics
```

Load balancer metrics:

- `xdp_l4lb_backend_packets_total`
- `xdp_l4lb_backend_bytes_total`
- `xdp_l4lb_backend_flows_total`
- `xdp_l4lb_datapath_events_total`

`redirect_requested` means the XDP program returned a redirect action. Final transmit failures should also be checked through kernel XDP redirect error tracepoints and interface counters.

Start the local observability stack with a non-empty Grafana admin password:

```bash
cd deploy
export GRAFANA_ADMIN_PASSWORD='replace-with-a-strong-password'
docker compose up -d
```

Prometheus and Grafana bind to localhost by default. Grafana automatically provisions the Prometheus datasource and dashboard.

## Operational boundaries

- IPv4 only
- TCP and UDP only
- DSR Layer 2 forwarding only
- Static configuration loaded at process start
- No active backend health checking
- IPv4 fragments are passed to the host stack
- The flow table is LRU-based and entries may be evicted under pressure
- XDP redirect success must be verified outside the program

