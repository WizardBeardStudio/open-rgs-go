#!/usr/bin/env bash
set -euo pipefail

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
root_dir="${RGS_SOAK_WORKDIR:-/tmp/open-rgs-go-soak}"
out_dir="${RGS_SOAK_OUT_DIR:-${root_dir}/${timestamp}}"
mkdir -p "${out_dir}"

runs="${RGS_SOAK_RUNS:-3}"
benchtime="${RGS_SOAK_BENCHTIME:-30s}"
cpu="${RGS_SOAK_CPU:-}"
bench_out="${out_dir}/benchmark_output.txt"
summary_out="${out_dir}/summary.json"

echo "running soak benchmarks (runs=${runs}, benchtime=${benchtime}, cpu=${cpu:-default})"
bench_cmd=(go test ./internal/platform/server -run '^$' -bench '^(BenchmarkLedgerDeposit|BenchmarkWageringPlaceWager)$' -benchmem -count "${runs}" -benchtime "${benchtime}")
if [[ -n "${cpu}" ]]; then
  bench_cmd+=(-cpu "${cpu}")
fi
"${bench_cmd[@]}" | tee "${bench_out}"

ledger_max_ns="$(awk '/^BenchmarkLedgerDeposit/ {if ($3+0 > max) max=$3+0} END {print max+0}' "${bench_out}")"
wager_max_ns="$(awk '/^BenchmarkWageringPlaceWager/ {if ($3+0 > max) max=$3+0} END {print max+0}' "${bench_out}")"

if [[ "${ledger_max_ns}" == "0" || "${wager_max_ns}" == "0" ]]; then
  echo "failed to parse benchmark output" >&2
  exit 1
fi

status="pass"
if [[ -n "${RGS_SOAK_LEDGER_DEPOSIT_NS_OP_MAX:-}" ]] && (( ledger_max_ns > RGS_SOAK_LEDGER_DEPOSIT_NS_OP_MAX )); then
  status="fail"
fi
if [[ -n "${RGS_SOAK_WAGER_PLACE_NS_OP_MAX:-}" ]] && (( wager_max_ns > RGS_SOAK_WAGER_PLACE_NS_OP_MAX )); then
  status="fail"
fi

cat >"${summary_out}" <<EOF
{
  "captured_at_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "runs": ${runs},
  "benchtime": "${benchtime}",
  "cpu": "${cpu:-default}",
  "ledger_deposit_max_ns_op": ${ledger_max_ns},
  "wager_place_max_ns_op": ${wager_max_ns},
  "ledger_deposit_ns_op_max_threshold": ${RGS_SOAK_LEDGER_DEPOSIT_NS_OP_MAX:-null},
  "wager_place_ns_op_max_threshold": ${RGS_SOAK_WAGER_PLACE_NS_OP_MAX:-null},
  "result": "${status}"
}
EOF

cat <<EOF
soak artifacts created:
  ${bench_out}
  ${summary_out}
EOF

if [[ "${status}" != "pass" ]]; then
  exit 1
fi
