#!/usr/bin/env bash
set -euo pipefail

# Runs load/soak qualification across predefined operator profile classes.
# Each profile maps to runs/benchtime/cpu/threshold settings and produces
# standalone artifacts plus an aggregate matrix summary.

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
root_dir="${RGS_SOAK_MATRIX_WORKDIR:-/tmp/open-rgs-go-soak-matrix}"
out_dir="${root_dir}/${timestamp}"
mkdir -p "${out_dir}"

profiles_csv="${RGS_SOAK_PROFILE_SET:-us-regulated-small,us-regulated-medium,us-regulated-large}"
IFS=',' read -r -a profiles <<<"${profiles_csv}"

matrix_summary="${out_dir}/matrix_summary.json"
matrix_rows=""
overall="pass"

profile_defaults() {
  case "$1" in
    us-regulated-small)
      echo "3 20s 1 90000 140000"
      ;;
    us-regulated-medium)
      echo "4 30s 2 110000 170000"
      ;;
    us-regulated-large)
      echo "5 45s 4 140000 220000"
      ;;
    *)
      return 1
      ;;
  esac
}

for raw_profile in "${profiles[@]}"; do
  profile="$(echo "${raw_profile}" | xargs)"
  if [[ -z "${profile}" ]]; then
    continue
  fi

  if ! defaults="$(profile_defaults "${profile}")"; then
    echo "unknown soak profile: ${profile}" >&2
    exit 1
  fi
  read -r runs benchtime cpu ledger_max wager_max <<<"${defaults}"

  profile_slug="${profile//[^a-zA-Z0-9._-]/_}"
  profile_dir="${out_dir}/${profile_slug}"
  mkdir -p "${profile_dir}"

  status="pass"
  if ! (
    RGS_SOAK_OUT_DIR="${profile_dir}" \
    RGS_SOAK_RUNS="${runs}" \
    RGS_SOAK_BENCHTIME="${benchtime}" \
    RGS_SOAK_CPU="${cpu}" \
    RGS_SOAK_LEDGER_DEPOSIT_NS_OP_MAX="${ledger_max}" \
    RGS_SOAK_WAGER_PLACE_NS_OP_MAX="${wager_max}" \
    ./scripts/load_soak_check.sh
  ); then
    status="fail"
    overall="fail"
  fi

  ledger_observed="$(awk -F': ' '/"ledger_deposit_max_ns_op"/ {gsub(/,/, "", $2); gsub(/ /, "", $2); print $2}' "${profile_dir}/summary.json")"
  wager_observed="$(awk -F': ' '/"wager_place_max_ns_op"/ {gsub(/,/, "", $2); gsub(/ /, "", $2); print $2}' "${profile_dir}/summary.json")"

  row=$(cat <<JSON
  {
    "profile": "${profile}",
    "runs": ${runs},
    "benchtime": "${benchtime}",
    "cpu": "${cpu}",
    "ledger_deposit_ns_op_max_threshold": ${ledger_max},
    "wager_place_ns_op_max_threshold": ${wager_max},
    "ledger_deposit_max_ns_op": ${ledger_observed:-0},
    "wager_place_max_ns_op": ${wager_observed:-0},
    "result": "${status}",
    "artifacts_dir": "${profile_dir}"
  }
JSON
)

  if [[ -n "${matrix_rows}" ]]; then
    matrix_rows+=$',\n'
  fi
  matrix_rows+="${row}"
done

cat >"${matrix_summary}" <<EOF_JSON
{
  "captured_at_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "profiles": [
${matrix_rows}
  ],
  "result": "${overall}"
}
EOF_JSON

cat <<EOF_OUT
soak profile matrix artifacts created:
  ${matrix_summary}
EOF_OUT

if [[ "${overall}" != "pass" ]]; then
  exit 1
fi
