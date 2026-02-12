#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${RGS_SOAK_DATABASE_URL:-}" ]]; then
  echo "RGS_SOAK_DATABASE_URL is required" >&2
  exit 1
fi

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
root_dir="${RGS_SOAK_DB_WORKDIR:-/tmp/open-rgs-go-soak-db}"
out_dir="${RGS_SOAK_OUT_DIR:-${root_dir}/${timestamp}}"
mkdir -p "${out_dir}"

runs="${RGS_SOAK_RUNS:-3}"
benchtime="${RGS_SOAK_BENCHTIME:-30s}"
cpu="${RGS_SOAK_CPU:-}"
bench_out="${out_dir}/benchmark_output.txt"
summary_out="${out_dir}/summary.json"

echo "running DB-backed soak benchmarks (runs=${runs}, benchtime=${benchtime}, cpu=${cpu:-default})"
bench_cmd=(go test ./internal/platform/server -run '^$' -bench '^(BenchmarkLedgerDepositPostgres|BenchmarkWageringPlaceWagerPostgres)$' -benchmem -count "${runs}" -benchtime "${benchtime}")
if [[ -n "${cpu}" ]]; then
  bench_cmd+=(-cpu "${cpu}")
fi
RGS_TEST_DATABASE_URL="${RGS_SOAK_DATABASE_URL}" "${bench_cmd[@]}" | tee "${bench_out}"

ledger_max_ns="$(awk '/^BenchmarkLedgerDepositPostgres/ {if ($3+0 > max) max=$3+0} END {print max+0}' "${bench_out}")"
wager_max_ns="$(awk '/^BenchmarkWageringPlaceWagerPostgres/ {if ($3+0 > max) max=$3+0} END {print max+0}' "${bench_out}")"

if [[ "${ledger_max_ns}" == "0" || "${wager_max_ns}" == "0" ]]; then
  echo "failed to parse benchmark output" >&2
  exit 1
fi

status="pass"
if [[ -n "${RGS_SOAK_DB_LEDGER_DEPOSIT_NS_OP_MAX:-}" ]] && (( ledger_max_ns > RGS_SOAK_DB_LEDGER_DEPOSIT_NS_OP_MAX )); then
  status="fail"
fi
if [[ -n "${RGS_SOAK_DB_WAGER_PLACE_NS_OP_MAX:-}" ]] && (( wager_max_ns > RGS_SOAK_DB_WAGER_PLACE_NS_OP_MAX )); then
  status="fail"
fi

cat >"${summary_out}" <<EOF_JSON
{
  "captured_at_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "runs": ${runs},
  "benchtime": "${benchtime}",
  "cpu": "${cpu:-default}",
  "ledger_deposit_postgres_max_ns_op": ${ledger_max_ns},
  "wager_place_postgres_max_ns_op": ${wager_max_ns},
  "ledger_deposit_postgres_ns_op_max_threshold": ${RGS_SOAK_DB_LEDGER_DEPOSIT_NS_OP_MAX:-null},
  "wager_place_postgres_ns_op_max_threshold": ${RGS_SOAK_DB_WAGER_PLACE_NS_OP_MAX:-null},
  "result": "${status}"
}
EOF_JSON

cat <<EOF_OUT
DB-backed soak artifacts created:
  ${bench_out}
  ${summary_out}
EOF_OUT

if [[ "${status}" != "pass" ]]; then
  exit 1
fi
