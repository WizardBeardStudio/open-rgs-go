#!/usr/bin/env bash
set -euo pipefail

# Collects Gate 10 evidence artifacts and writes a checklist-ready summary.
# By default this script is non-strict and returns success even when steps are
# skipped or fail. Set RGS_GATE10_FAIL_ON_INCOMPLETE=true to enforce all-pass.

ts="$(date -u +%Y%m%dT%H%M%SZ)"
root_dir="${RGS_GATE10_WORKDIR:-/tmp/open-rgs-go-gate10}"
run_dir="${root_dir}/${ts}"
logs_dir="${run_dir}/logs"
mkdir -p "${logs_dir}"

fail_on_incomplete="${RGS_GATE10_FAIL_ON_INCOMPLETE:-false}"
gocache="${RGS_GATE10_GOCACHE:-/tmp/open-rgs-go-gocache}"
skip_soak_db="${RGS_GATE10_SKIP_SOAK_DB:-false}"
skip_soak_matrix="${RGS_GATE10_SKIP_SOAK_MATRIX:-false}"
skip_keyset="${RGS_GATE10_SKIP_KEYSET:-false}"

soak_db_status="not_run"
soak_matrix_status="not_run"
keyset_status="not_run"

soak_db_log="${logs_dir}/soak_db.log"
soak_matrix_log="${logs_dir}/soak_matrix.log"
keyset_log="${logs_dir}/keyset.log"

soak_db_out_dir="${run_dir}/soak-db"
soak_matrix_workdir="${run_dir}/soak-matrix"
keyset_workdir="${run_dir}/keyset"
keyset_event_id="${RGS_KEYSET_EVENT_ID:-gate10-keyset-${ts}}"

soak_db_artifact_dir=""
soak_matrix_artifact_dir=""
keyset_artifact_dir=""

run_step() {
  local log_file="$1"
  shift
  set +e
  "$@" >"${log_file}" 2>&1
  local status=$?
  set -e
  return "${status}"
}

if [[ "${skip_soak_db}" == "true" ]]; then
  echo "skipped: RGS_GATE10_SKIP_SOAK_DB=true" >"${soak_db_log}"
  soak_db_status="skipped"
elif [[ -n "${RGS_SOAK_DATABASE_URL:-}" ]]; then
  mkdir -p "${soak_db_out_dir}"
  if run_step "${soak_db_log}" env GOCACHE="${gocache}" RGS_SOAK_OUT_DIR="${soak_db_out_dir}" RGS_SOAK_DATABASE_URL="${RGS_SOAK_DATABASE_URL}" ./scripts/load_soak_check_db.sh; then
    soak_db_status="pass"
    soak_db_artifact_dir="${soak_db_out_dir}"
  else
    soak_db_status="fail"
    soak_db_artifact_dir="${soak_db_out_dir}"
  fi
else
  {
    echo "skipped: RGS_SOAK_DATABASE_URL is not set"
    echo "set RGS_SOAK_DATABASE_URL and rerun"
  } >"${soak_db_log}"
  soak_db_status="skipped"
fi

if [[ "${skip_soak_matrix}" == "true" ]]; then
  echo "skipped: RGS_GATE10_SKIP_SOAK_MATRIX=true" >"${soak_matrix_log}"
  soak_matrix_status="skipped"
else
  mkdir -p "${soak_matrix_workdir}"
  if run_step "${soak_matrix_log}" env GOCACHE="${gocache}" RGS_SOAK_MATRIX_WORKDIR="${soak_matrix_workdir}" ./scripts/load_soak_matrix.sh; then
    soak_matrix_status="pass"
  else
    soak_matrix_status="fail"
  fi
  if [[ -d "${soak_matrix_workdir}" ]]; then
    latest_matrix="$(ls -1dt "${soak_matrix_workdir}"/* 2>/dev/null | head -n1 || true)"
    if [[ -n "${latest_matrix}" ]]; then
      soak_matrix_artifact_dir="${latest_matrix}"
    fi
  fi
fi

if [[ "${skip_keyset}" == "true" ]]; then
  echo "skipped: RGS_GATE10_SKIP_KEYSET=true" >"${keyset_log}"
  keyset_status="skipped"
elif [[ -n "${RGS_JWT_KEYSET_FILE:-}" || -n "${RGS_JWT_KEYSET_COMMAND:-}" ]]; then
  mkdir -p "${keyset_workdir}"
  if run_step "${keyset_log}" env RGS_KEYSET_WORKDIR="${keyset_workdir}" RGS_KEYSET_EVENT_ID="${keyset_event_id}" RGS_JWT_KEYSET_FILE="${RGS_JWT_KEYSET_FILE:-}" RGS_JWT_KEYSET_COMMAND="${RGS_JWT_KEYSET_COMMAND:-}" RGS_KEYSET_PREVIOUS_SUMMARY_FILE="${RGS_KEYSET_PREVIOUS_SUMMARY_FILE:-}" ./scripts/keyset_rotation_evidence.sh; then
    keyset_status="pass"
  else
    keyset_status="fail"
  fi
  if [[ -d "${keyset_workdir}/${keyset_event_id}" ]]; then
    keyset_artifact_dir="${keyset_workdir}/${keyset_event_id}"
  fi
else
  {
    echo "skipped: neither RGS_JWT_KEYSET_FILE nor RGS_JWT_KEYSET_COMMAND is set"
    echo "set one keyset source and rerun"
  } >"${keyset_log}"
  keyset_status="skipped"
fi

overall="pass"
if [[ "${soak_db_status}" != "pass" || "${soak_matrix_status}" != "pass" || "${keyset_status}" != "pass" ]]; then
  overall="incomplete"
fi

summary_json="${run_dir}/gate10_summary.json"
summary_md="${run_dir}/gate10_checklist_snippet.md"

cat >"${summary_json}" <<EOF
{
  "captured_at_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "run_dir": "${run_dir}",
  "overall_result": "${overall}",
  "steps": {
    "soak_qual_db": {
      "status": "${soak_db_status}",
      "artifact_dir": "${soak_db_artifact_dir}",
      "log_file": "${soak_db_log}"
    },
    "soak_qual_matrix": {
      "status": "${soak_matrix_status}",
      "artifact_dir": "${soak_matrix_artifact_dir}",
      "log_file": "${soak_matrix_log}"
    },
    "keyset_evidence": {
      "status": "${keyset_status}",
      "artifact_dir": "${keyset_artifact_dir}",
      "log_file": "${keyset_log}"
    }
  }
}
EOF

cat >"${summary_md}" <<EOF
# Gate 10 Evidence Snapshot (${ts})

- Overall result: "${overall}"
- Run directory: "${run_dir}"

## Gate 10 Mapping (README Section 13)

1. In-memory mirrors closure evidence
- DB soak (make soak-qual-db): "${soak_db_status}"
- Matrix soak (make soak-qual-matrix): "${soak_matrix_status}"
- DB soak artifacts: "${soak_db_artifact_dir}"
- Matrix soak artifacts: "${soak_matrix_artifact_dir}"

2. External key custody closure evidence
- Keyset rotation evidence (make keyset-evidence): "${keyset_status}"
- Keyset artifacts: "${keyset_artifact_dir}"

3. Promotions/UI scope closure evidence
- Attach implementation evidence or deferred-scope acceptance package from Gate 8.

## Logs

- DB soak log: "${soak_db_log}"
- Matrix soak log: "${soak_matrix_log}"
- Keyset log: "${keyset_log}"
EOF

cat <<EOF
gate10 evidence collection completed
  summary json: ${summary_json}
  checklist snippet: ${summary_md}
EOF

if [[ "${fail_on_incomplete}" == "true" && "${overall}" != "pass" ]]; then
  exit 1
fi
