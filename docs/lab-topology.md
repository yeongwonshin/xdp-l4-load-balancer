# Lab Topology

## Minimal layout

A practical namespace-based lab separates the client, load balancer ingress, backend egress network, and backend hosts.

```text
client namespace
    |
    | client veth
    |
load balancer ingress interface
    | xdp program attached here
load balancer backend bridge or egress interface
    |
    +-- backend one namespace
    |
    +-- backend two namespace
```

The backend `ifname` and `ifindex` values in the load balancer configuration refer to interfaces visible from the load balancer process. They do not refer to interface names inside the backend namespaces.

## Addressing example

```text
client              10.20.0.10
vip                 10.10.0.100
backend one         10.10.0.11
backend two         10.10.0.12
load balancer in    interface receiving client traffic
load balancer out   interface or bridge reaching backend mac addresses
```

The VIP does not need to be configured on the load balancer interface for the XDP map lookup. It must be routed or delivered to the interface where XDP is attached.

## Backend preparation

Run inside each backend host or namespace:

```bash
sudo ./scripts/setup-dsr-backend.sh 10.10.0.100
sudo python3 -m http.server 80 --bind 0.0.0.0
```

When the repository is not mounted inside a namespace, apply the equivalent address and sysctl commands manually.

Verify the VIP and ARP policy:

```bash
ip -4 address show dev lo
sysctl net.ipv4.conf.all.arp_ignore
sysctl net.ipv4.conf.all.arp_announce
```

## Load balancer startup

Use the egress interface that can transmit directly to the configured backend MAC addresses.

```bash
make build
sudo ./bin/xdp-l4lb \
  -iface lb-ingress \
  -config configs/example.yaml \
  -xdp-mode generic \
  -metrics :2112
```

Update `configs/example.yaml` so every backend entry contains the correct backend MAC and load balancer egress interface.

## Traffic checks

From the client namespace:

```bash
curl --connect-timeout 2 http://10.10.0.100/
```

On the load balancer:

```bash
curl -s http://127.0.0.1:2112/metrics | grep xdp_l4lb
ip -s link show dev lb-ingress
ip -s link show dev lb-egress
```

On each backend:

```bash
sudo tcpdump -eni any host 10.10.0.100 and tcp port 80
```

The received frame should retain destination IP `10.10.0.100`, use the selected backend destination MAC, and use the load balancer egress interface MAC as the source MAC.

## Affinity test

A single TCP connection should stay on one backend. Generate multiple independent connections to exercise distribution:

```bash
for request in $(seq 1 20); do
  curl -s --no-keepalive http://10.10.0.100/ >/dev/null
done
```

Client source port reuse, connection pooling, and HTTP keep-alive can reduce the number of unique five tuples. Confirm the `xdp_l4lb_backend_flows_total` metric when interpreting distribution.

## Pass-through tests

Send traffic that should not be redirected and verify that it reaches the host stack:

- a different VIP
- a different destination port
- ICMP
- IPv6
- fragmented IPv4

The matching datapath event counters should increase for non-IPv4, fragmented, unsupported transport, or service-miss outcomes.
