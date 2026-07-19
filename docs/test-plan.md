# Test Plan

## Automated checks

Run the complete local check set:

```bash
make check
```

The automated tests cover:

- strict YAML field handling
- default and normalized service values
- duplicate service key rejection
- invalid multicast MAC rejection
- single-document YAML enforcement
- IPv4 map key byte order
- transport port map key byte order
- TCP and UDP protocol conversion
- case-insensitive XDP attach mode parsing

`make check` also runs formatting verification and `go vet`. BPF generation is a prerequisite, so clang, libbpf headers, and Linux headers must be installed.

## Configuration checks

```bash
make build
./bin/xdp-l4lb -config configs/example.yaml -check-config
```

Negative cases should include:

- unknown YAML field
- empty service list
- duplicate service name
- duplicate VIP, port, and protocol tuple
- zero port
- unsupported protocol or mode
- empty backend list
- invalid or multicast backend MAC
- missing egress interface reference
- inconsistent `ifname` and `ifindex`
- service or backend count above map limits

Interface resolution is performed when maps are programmed, so invalid operating-system interface references are detected during startup rather than the YAML-only check.

## Datapath integration

1. Attach in generic mode.
2. Send TCP traffic to a configured VIP and port.
3. Confirm the backend receives frames with the VIP unchanged.
4. Confirm the Ethernet destination is the backend MAC.
5. Confirm the Ethernet source is the configured egress interface MAC.
6. Confirm repeated packets with the same five tuple use the same backend.
7. Confirm multiple flows distribute across the configured backend range.
8. Repeat with a UDP service.
9. Repeat with one and two VLAN tags when the environment supports them.
10. Confirm non-service traffic reaches the host stack.
11. Confirm IPv4 fragments reach the host stack.
12. Confirm malformed length fields do not cause verifier or runtime failures.

## Observability checks

```bash
curl -fsS http://127.0.0.1:2112/healthz
curl -fsS http://127.0.0.1:2112/readyz
curl -fsS http://127.0.0.1:2112/metrics
```

Validate that:

- backend packet and byte counters increase
- new flow counters increase only for successful first insertions
- service miss counters increase for unconfigured VIP traffic
- fragmented counters increase for fragmented IPv4 traffic
- redirect request counters increase for selected backend traffic
- process and Go runtime metrics are present

## Redirect verification

Application metrics count redirect requests, not final device transmission. During integration tests also inspect:

- egress interface packet and drop counters
- backend packet capture
- XDP redirect error tracepoints
- driver logs

## Performance checks

Use `wrk`, `iperf3`, `pktgen`, MoonGen, or another reproducible traffic generator. Compare generic and driver modes while recording:

- packets per second
- throughput
- latency distribution
- CPU utilization by core
- flow table pressure
- redirect failures
- counter collection overhead

Test both a small backend set and a backend set large enough to exercise cache behavior. Include repeated five tuples and high-cardinality flow traffic.

## Shutdown checks

Send `sigint` and `sigterm` separately. Verify that:

- the HTTP listener closes
- the XDP link detaches
- the process exits without leaving a pinned object or stale attachment
- subsequent startup succeeds on the same interface
