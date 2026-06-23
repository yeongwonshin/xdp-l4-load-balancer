# BPF Maps

## `services`

Type: `BPF_MAP_TYPE_HASH`

Key: `service_key`

- `vip`: IPv4 VIP, network byte order
- `port`: TCP/UDP destination port, network byte order
- `proto`: `6` TCP or `17` UDP

Value: `service_value`

- `backend_start`: first backend ID for this service
- `backend_count`: number of backends
- `flags`: currently `LB_FLAG_DSR`

## `backends`

Type: `BPF_MAP_TYPE_ARRAY`

Key: backend ID

Value:

- backend IPv4 address
- egress interface index
- backend destination MAC

## `backend_stats`

Type: `BPF_MAP_TYPE_ARRAY`

Counters:

- packets
- bytes
- flows

## `flow_table`

Type: `BPF_MAP_TYPE_LRU_HASH`

Key: 5-tuple

Value:

- selected backend ID
- last seen timestamp

LRU map을 사용하므로 오래된 flow entry는 pressure 상황에서 자동 eviction될 수 있습니다.

## `lb_config`

Type: `BPF_MAP_TYPE_ARRAY`

Index `0`에 load balancer interface MAC과 기본 action을 저장합니다.
