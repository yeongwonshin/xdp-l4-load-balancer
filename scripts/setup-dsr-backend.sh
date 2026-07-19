#!/usr/bin/env bash
set -euo pipefail

vip=${1:?usage: setup-dsr-backend.sh vip optional-device}
device=${2:-lo}

if ! command -v ip >/dev/null 2>&1; then
  echo "ip command is required" >&2
  exit 1
fi

if ! ip -4 address show dev "${device}" >/dev/null 2>&1; then
  echo "device not found: ${device}" >&2
  exit 1
fi

if ! [[ ${vip} =~ ^([0-9]{1,3}\.){3}[0-9]{1,3}$ ]]; then
  echo "invalid ipv4 address: ${vip}" >&2
  exit 1
fi

IFS=. read -r octet1 octet2 octet3 octet4 extra <<<"${vip}"
if [[ -n ${extra:-} || -z ${octet1:-} || -z ${octet2:-} || -z ${octet3:-} || -z ${octet4:-} ]]; then
  echo "invalid ipv4 address: ${vip}" >&2
  exit 1
fi
for octet in "${octet1}" "${octet2}" "${octet3}" "${octet4}"; do
  if ! [[ ${octet} =~ ^[0-9]+$ ]] || ((10#${octet} > 255)); then
    echo "invalid ipv4 address: ${vip}" >&2
    exit 1
  fi
done

if ! ip -4 address show dev "${device}" | grep -Fq "inet ${vip}/32"; then
  sudo ip address add "${vip}/32" dev "${device}"
fi

sudo sysctl -w net.ipv4.conf.all.arp_ignore=1
sudo sysctl -w net.ipv4.conf.all.arp_announce=2
sudo sysctl -w net.ipv4.conf.default.arp_ignore=1
sudo sysctl -w net.ipv4.conf.default.arp_announce=2

echo "dsr backend prepared for vip ${vip} on ${device}"
