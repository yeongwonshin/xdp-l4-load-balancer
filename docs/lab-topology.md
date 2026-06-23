# Lab Topology

가장 단순한 테스트 토폴로지는 다음과 같습니다.

```text
client namespace
    |
    | veth
    |
load balancer namespace / host
    | XDP attached on ingress interface
    |
    | bridge or veth pair
    |
backend namespaces
```

## Backend requirements for DSR

각 backend는 VIP를 local address로 가지고 있어야 합니다.

```bash
ip addr add 10.10.0.100/32 dev lo
sysctl -w net.ipv4.conf.all.arp_ignore=1
sysctl -w net.ipv4.conf.all.arp_announce=2
```

## Traffic test

```bash
# backend
python3 -m http.server 80

# client
curl http://10.10.0.100/

# metrics
curl http://LOAD_BALANCER_IP:2112/metrics
```
