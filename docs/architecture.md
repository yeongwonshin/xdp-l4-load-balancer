# Architecture

## Goal

XDP 단계에서 L4 load balancing decision을 내려 kernel network stack 진입 전 packet을 backend로 redirect합니다.

## Data path

1. Ethernet frame bounds check
2. IPv4 packet check
3. TCP/UDP header parse
4. `services` map lookup
5. `flow_table` map lookup
6. Missing flow이면 5-tuple hash로 backend 선택
7. backend stats update
8. DSR mode L2 rewrite
9. `bpf_redirect(ifindex, 0)`

## Control plane

Go control plane은 다음 역할을 담당합니다.

- YAML config loading
- VIP/service/backend validation
- eBPF object loading
- XDP attach/detach lifecycle management
- BPF map programming
- Prometheus metrics serving

## DSR mode

이 skeleton은 DSR/L2 mode를 기본으로 둡니다.

- VIP destination IP는 변경하지 않습니다.
- Ethernet destination MAC을 selected backend MAC으로 변경합니다.
- Ethernet source MAC을 load balancer interface MAC으로 변경합니다.
- backend는 VIP를 loopback/dummy interface에 가지고 있어야 합니다.
- backend의 ARP flux를 막기 위해 `arp_ignore`, `arp_announce` 설정이 필요합니다.

## Failure behavior

- service map miss: `XDP_PASS`
- unsupported protocol: `XDP_PASS`
- malformed packet: `XDP_PASS`
- backend map miss: `XDP_PASS`
- redirect failure: kernel tracepoint에서 확인 가능
