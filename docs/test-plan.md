# Test Plan

## Unit-level checks

- YAML config validation
- IPv4 / port / protocol parsing
- service/backend ID assignment
- Prometheus collector label generation

## Integration checks

1. Attach XDP in generic mode.
2. Send TCP traffic to VIP:80.
3. Confirm backend receives packet.
4. Confirm same 5-tuple sticks to same backend.
5. Confirm UDP service lookup works.
6. Confirm map miss traffic is passed to host stack.
7. Confirm `/metrics` exposes packets/bytes/flows.

## Performance checks

- `wrk`, `iperf3`, `pktgen`, or MoonGen traffic generation
- Compare generic XDP vs driver XDP
- Measure packets per second and CPU utilization
- Observe redirect error tracepoints if packets are not delivered

## Safety checks

- malformed IP header should not crash/verifier reject
- unsupported protocol should pass
- empty backend list should be rejected by control plane
- invalid MAC/ifname should be rejected by control plane
