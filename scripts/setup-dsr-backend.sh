#!/usr/bin/env bash
set -euo pipefail

VIP=${1:?usage: setup-dsr-backend.sh <vip>}

sudo ip addr add "${VIP}/32" dev lo 2>/dev/null || true
sudo sysctl -w net.ipv4.conf.all.arp_ignore=1
sudo sysctl -w net.ipv4.conf.all.arp_announce=2
sudo sysctl -w net.ipv4.conf.default.arp_ignore=1
sudo sysctl -w net.ipv4.conf.default.arp_announce=2

echo "DSR backend prepared for VIP ${VIP}"
