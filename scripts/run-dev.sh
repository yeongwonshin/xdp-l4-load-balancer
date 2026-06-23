#!/usr/bin/env bash
set -euo pipefail

IFACE=${IFACE:-eth0}
CONFIG=${CONFIG:-configs/example.yaml}
XDP_MODE=${XDP_MODE:-generic}

make build
sudo ./bin/xdp-l4lb -iface "$IFACE" -config "$CONFIG" -xdp-mode "$XDP_MODE" -metrics :2112
