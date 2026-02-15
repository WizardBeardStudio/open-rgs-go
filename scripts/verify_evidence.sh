#!/usr/bin/env bash
set -euo pipefail

ts="$(date -u +%Y%m%dT%H%M%SZ)"
base_dir="${RGS_VERIFY_EVIDENCE_DIR:-artifacts/verify}"
run_dir="${base_dir}/${ts}"
proto_mode="${RGS_VERIFY_EVIDENCE_PROTO_MODE:-full}"
git_commit="$(git rev-parse HEAD)"
git_branch="$(git rev-parse --abbrev-ref HEAD)"
ci_run_id="${GITHUB_RUN_ID:-}"
ci_run_attempt="${GITHUB_RUN_ATTEMPT:-}"
ci_ref="${GITHUB_REF:-}"
ci_sha="${GITHUB_SHA:-}"
if command -v hostname >/dev/null 2>&1; then
  host_name="$(hostname)"
else
  host_name="$(uname -n)"
fi
os_name="$(uname -s)"
arch_name="$(uname -m)"
go_version="$(go version | sed 's/"/\\"/g')"
buf_version="$(buf --version 2>/dev/null | sed 's/"/\\"/g' || true)"

mkdir -p "${run_dir}"

run_and_capture() {
  local cmd="$1"
  local log_file="$2"

  set +e
  bash -lc "${cmd}" >"${log_file}" 2>&1
  local status=$?
  set -e
  return "${status}"
}

proto_log="${run_dir}/proto_check.log"
verify_log="${run_dir}/make_verify.log"
summary_file="${run_dir}/summary.json"
manifest_file="${run_dir}/manifest.sha256"
latest_file="${base_dir}/LATEST"

proto_cmd="RGS_PROTO_CHECK_MODE=${proto_mode} make proto-check"
verify_cmd="RGS_PROTO_CHECK_MODE=${proto_mode} make verify"

proto_status=0
verify_status=0

set +e
run_and_capture "${proto_cmd}" "${proto_log}"
proto_status=$?
run_and_capture "${verify_cmd}" "${verify_log}"
verify_status=$?
set -e

cat >"${summary_file}" <<EOF
{
  "timestamp_utc": "${ts}",
  "git_commit": "${git_commit}",
  "git_branch": "${git_branch}",
  "ci_run_id": "${ci_run_id}",
  "ci_run_attempt": "${ci_run_attempt}",
  "ci_ref": "${ci_ref}",
  "ci_sha": "${ci_sha}",
  "hostname": "${host_name}",
  "os": "${os_name}",
  "arch": "${arch_name}",
  "go_version": "${go_version}",
  "buf_version": "${buf_version}",
  "proto_check_command": "${proto_cmd}",
  "make_verify_command": "${verify_cmd}",
  "proto_check_status": ${proto_status},
  "make_verify_status": ${verify_status},
  "overall_status": $([[ ${proto_status} -eq 0 && ${verify_status} -eq 0 ]] && echo "\"pass\"" || echo "\"fail\"")
}
EOF

checksum_file() {
  local file="$1"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum "${file}"
    return 0
  fi
  if command -v shasum >/dev/null 2>&1; then
    shasum -a 256 "${file}"
    return 0
  fi
  return 1
}

{
  checksum_file "${proto_log}" || { echo "no sha256 tool available" >&2; exit 1; }
  checksum_file "${verify_log}" || { echo "no sha256 tool available" >&2; exit 1; }
  checksum_file "${summary_file}" || { echo "no sha256 tool available" >&2; exit 1; }
} >"${manifest_file}"

printf '%s\n' "${run_dir}" >"${latest_file}"

if [[ ${proto_status} -ne 0 || ${verify_status} -ne 0 ]]; then
  echo "verification evidence failed; see ${summary_file}" >&2
  exit 1
fi

echo "verification evidence captured at ${run_dir}"
