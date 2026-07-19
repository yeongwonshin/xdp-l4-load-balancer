#!/usr/bin/env bash
set -euo pipefail

root_dir=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
output_dir="${root_dir}/cmd/xdp-l4lb"
clang_bin=${BPF_CLANG:-clang}

case "$(go env GOARCH)" in
  amd64)
    target_arch=x86
    ;;
  arm64)
    target_arch=arm64
    ;;
  *)
    echo "unsupported go architecture for bpf generation: $(go env GOARCH)" >&2
    exit 1
    ;;
esac

include_args=(
  "-I${root_dir}/bpf"
  "-I/usr/include"
)

multiarch=$(${clang_bin} -print-multiarch 2>/dev/null || true)
if [[ -n "${multiarch}" && -d "/usr/include/${multiarch}" ]]; then
  include_args+=("-I/usr/include/${multiarch}")
fi

cd "${output_dir}"
exec go run github.com/cilium/ebpf/cmd/bpf2go \
  -cc "${clang_bin}" \
  -no-strip \
  -cflags "-O2 -g -Wall -Werror -D__TARGET_ARCH_${target_arch}" \
  bpf "${root_dir}/bpf/xdp_l4lb.bpf.c" -- "${include_args[@]}"
