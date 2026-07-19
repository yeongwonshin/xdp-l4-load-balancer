# Operations and Troubleshooting

## Host preparation

Install a compiler with a BPF backend, libbpf development headers, Linux UAPI headers, architecture-specific include files, and Go. Confirm the expected files and tools exist:

```bash
command -v clang
test -f /usr/include/bpf/bpf_helpers.h
test -f /usr/include/linux/bpf.h
go version
```

The generator detects amd64 and arm64 from `go env GOARCH`. Override the compiler when needed:

```bash
make build BPF_CLANG=clang-18
```

## Preflight workflow

```bash
make format-check
make test
make vet
make build
./bin/xdp-l4lb -config configs/example.yaml -check-config
```

Before attaching XDP, verify every backend egress reference on the load balancer host:

```bash
ip -br link
ip link show dev eth1
```

When a backend specifies both `ifname` and `ifindex`, startup fails unless they resolve to the same interface.

## DSR network requirements

The load balancer forwards the original VIP at Layer 2. Each backend must:

- configure the VIP as a local `/32` address
- suppress ARP announcements for the VIP on the service-facing network
- have a route that sends responses directly to the client or upstream router
- accept traffic whose destination is the local VIP
- allow the service port through its firewall

Example:

```bash
sudo ./scripts/setup-dsr-backend.sh 10.10.0.100
```

The script is idempotent for the address and applies the common ARP suppression settings. Review sysctl policy before using it on a shared host.

## Attach mode selection

Use generic mode first:

```bash
sudo ./bin/xdp-l4lb -iface eth0 -config configs/example.yaml -xdp-mode generic
```

After functional validation, try driver mode:

```bash
sudo ./bin/xdp-l4lb -iface eth0 -config configs/example.yaml -xdp-mode driver
```

Hardware mode depends on NIC firmware, driver support, and offload restrictions. An attach failure does not automatically fall back to another mode.

## Service startup

A system service should run the binary with an explicit configuration path, interface, attach mode, and metrics address. Grant only the privileges required by the deployment. Root is simplest for a lab, but production deployments should evaluate Linux capabilities and system service hardening.

The process loads maps in memory and does not pin them. Restarting the process recreates maps and resets affinity and counters. Configuration changes require a restart.

## Metrics exposure

Binding `-metrics :2112` exposes endpoints on every local address. Use a loopback bind when only a local scraper is needed:

```bash
sudo ./bin/xdp-l4lb \
  -iface eth0 \
  -config configs/example.yaml \
  -xdp-mode driver \
  -metrics 127.0.0.1:2112
```

The bundled Prometheus container accesses the host through `host.docker.internal`, so the load balancer must listen on an address reachable from that container. Protect the endpoint with host firewall policy or a trusted network boundary because it has no application-layer authentication.

## Dashboard stack

```bash
cd deploy
export GRAFANA_ADMIN_PASSWORD='replace-with-a-strong-password'
docker compose config
docker compose up -d
```

The compose stack binds Prometheus and Grafana to localhost, keeps persistent data in named volumes, and provisions both the Prometheus datasource and the load balancer dashboard.

Check status:

```bash
docker compose ps
docker compose logs prometheus
docker compose logs grafana
```

## Common startup failures

### Missing bpf headers

Symptom: clang reports that `bpf/bpf_helpers.h` cannot be found.

Action: install the libbpf development package and confirm `/usr/include/bpf/bpf_helpers.h` exists.

### Missing architecture headers

Symptom: an include such as `asm/types.h` cannot be found.

Action: install the architecture-specific libc development headers. The generator automatically adds clang's reported multiarch include directory when it exists.

### Memlock or permission error

Symptom: loading maps or programs fails with an operation-not-permitted or memory-lock error.

Action: run with sufficient privilege, verify kernel BPF policy, and inspect container or system service capability restrictions.

### Driver mode attach failure

Symptom: generic mode works but driver mode fails.

Action: verify native XDP support for the NIC driver and check whether another XDP program is attached.

```bash
ip -details link show dev eth0
```

### Backend interface resolution failure

Symptom: startup reports an unknown `ifname`, unknown `ifindex`, or mismatch.

Action: update the configuration to identify an existing Ethernet interface on the load balancer host. Backend `ifname` refers to the load balancer egress interface, not the interface name inside the backend host.

### Service miss for valid traffic

Check the exact VIP, port, and protocol in the configuration. Confirm the packet is IPv4, is not fragmented, and arrives on the interface where XDP is attached. Inspect:

```bash
curl -s http://127.0.0.1:2112/metrics | grep xdp_l4lb_datapath_events_total
```

A growing `service_miss` value indicates that parsing succeeded but the service key was not found or its backend range was invalid.

### Redirect requests without backend traffic

A growing `redirect_requested` counter only confirms that the program returned `xdp_redirect`. Verify:

```bash
ip -s link show dev eth1
tcpdump -eni eth1 host 10.10.0.100
```

Also inspect XDP redirect error tracepoints and ensure that the backend MAC is reachable through the configured egress interface.

### Responses do not return through the load balancer

That is expected in DSR mode. The backend responds directly. Verify the backend route to the client, reverse-path filtering policy, firewall rules, and upstream routing.

## Safe rollback

Stop the process cleanly with `sigterm` or `sigint`. The link handle detaches the XDP program as the process exits. Confirm the interface no longer shows an attached XDP program:

```bash
ip -details link show dev eth0
```

Avoid killing the process during map programming when possible. The maps are not pinned, so they are released when the process exits.
