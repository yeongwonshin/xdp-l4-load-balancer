# eBPF/XDP L4 Load Balancer

포트폴리오용 XDP 기반 L4 로드밸런서 골격입니다. XDP hook에서 TCP/UDP 패킷을 파싱하고, `VIP:Port:Protocol` 서비스 맵을 조회한 뒤 5-tuple hash로 backend를 선택합니다. 선택된 backend는 flow table에 저장되어 같은 connection이 같은 backend로 유지됩니다.

> 현재 구현은 DSR/L2 redirect 모드입니다. 즉, VIP 목적지 IP는 유지하고 Ethernet destination MAC만 backend MAC으로 바꾼 뒤 `bpf_redirect()`로 egress interface에 전달합니다. backend는 loopback 또는 dummy interface에 VIP를 가지고 있어야 합니다.

## 주요 기능

- XDP 프로그램으로 Ethernet/IPv4/TCP/UDP 패킷 파싱
- `VIP + Port + Protocol` 기반 service lookup
- 5-tuple hash 기반 backend 선택
- LRU flow table로 connection affinity 유지
- TCP/UDP packet L2 rewrite + redirect
- backend별 packets/bytes/flows counter
- Go userspace control plane
- `/metrics` Prometheus exporter
- Grafana dashboard skeleton

## 아키텍처

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

## 디렉토리 구조

```text
xdp-l4-lb/
├── bpf/                         # eBPF/XDP datapath
│   ├── common.h                  # shared map/value struct definitions
│   └── xdp_l4lb.bpf.c            # XDP packet parser, hashing, redirect
├── cmd/xdp-l4lb/                 # Go control plane
│   ├── main.go                   # load/attach XDP, metrics server
│   ├── config.go                 # YAML config parser
│   ├── maps.go                   # service/backend map programming
│   ├── metrics.go                # Prometheus collector
│   └── types.go                  # Go-side map structs
├── configs/
│   └── example.yaml              # sample VIP/backend config
├── deploy/
│   ├── docker-compose.yml        # Prometheus + Grafana
│   ├── prometheus.yml
│   └── grafana/dashboards/
│       └── xdp-l4lb.json
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

## 빌드

필요 도구:

- Linux kernel with eBPF/XDP support
- clang/llvm
- bpftool 또는 kernel headers
- Go 1.22+
- root 권한 또는 필요한 capability

```bash
make build
```

`go generate`가 `bpf2go`를 실행해 eBPF object와 Go loader binding을 생성합니다.

## 실행

```bash
sudo ./bin/xdp-l4lb \
  -iface eth0 \
  -config configs/example.yaml \
  -xdp-mode generic \
  -metrics :2112
```

운영/성능 테스트에서는 `-xdp-mode driver`를 우선 고려하세요. NIC driver가 native XDP를 지원하지 않으면 `generic`으로 시작할 수 있습니다.

## Metrics

```bash
curl localhost:2112/metrics
```

노출 metric:

- `xdp_l4lb_backend_packets_total`
- `xdp_l4lb_backend_bytes_total`
- `xdp_l4lb_backend_flows_total`

Prometheus/Grafana:

```bash
cd deploy
 docker compose up -d
```

## DSR backend 설정 예시

backend 서버에서 VIP를 loopback에 올리고 ARP 응답을 억제합니다.

```bash
sudo ./scripts/setup-dsr-backend.sh 10.10.0.100
```

## 포트폴리오 확장 과제

1. NAT mode 추가: destination IP rewrite + checksum update
2. consistent hashing 또는 Maglev hashing
3. health checker와 동적 backend 제거
4. control plane REST API
5. pinned map 기반 무중단 reload
6. XDP_DROP 기반 ACL / SYN flood mitigation
7. tc egress hook 연동으로 SNAT/return path 처리
