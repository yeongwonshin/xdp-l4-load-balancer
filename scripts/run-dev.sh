#!/usr/bin/env bash
set -euo pipefail

iface=${IFACE:-eth0}
config=${CONFIG:-configs/example.yaml}
xdp_mode=${XDP_MODE:-generic}
metrics_addr=${METRICS_ADDR:-:2112}

make build
exec sudo ./bin/xdp-l4lb \
  -iface "${iface}" \
  -config "${config}" \
  -xdp-mode "${xdp_mode}" \
  -metrics "${metrics_addr}"
