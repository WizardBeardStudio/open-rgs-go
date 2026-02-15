#!/usr/bin/env bash
set -euo pipefail

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
root_dir="${RGS_PERF_WORKDIR:-/tmp/open-rgs-go-perf}"
out_dir="${root_dir}/${timestamp}"
mkdir -p "${out_dir}"

bench_out="${out_dir}/benchmark_output.txt"
summary_out="${out_dir}/summary.txt"

echo "running benchmark suite"
go test ./internal/platform/server -run '^$' -bench '^BenchmarkLedgerDeposit$' -benchmem | tee "${bench_out}"

deposit_ns="$(awk '/BenchmarkLedgerDeposit/ {print $(NF-2)}' "${bench_out}" | tail -n1)"
if [[ -z "${deposit_ns}" ]]; then
  echo "failed to parse BenchmarkLedgerDeposit ns/op" >&2
  exit 1
fi

{
  echo "timestamp=${timestamp}"
  echo "benchmark=BenchmarkLedgerDeposit"
  echo "ns_op=${deposit_ns}"
} >"${summary_out}"

if [[ -n "${RGS_PERF_LEDGER_DEPOSIT_NS_OP_MAX:-}" ]]; then
  if (( deposit_ns > RGS_PERF_LEDGER_DEPOSIT_NS_OP_MAX )); then
    echo "performance SLO breach: deposit ns/op=${deposit_ns} > ${RGS_PERF_LEDGER_DEPOSIT_NS_OP_MAX}" >&2
    exit 1
  fi
fi

cat <<EOF
performance artifacts created:
  ${bench_out}
  ${summary_out}
EOF
