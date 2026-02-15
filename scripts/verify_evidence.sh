#!/usr/bin/env bash
set -euo pipefail

ts="$(date -u +%Y%m%dT%H%M%SZ)"
summary_schema_version="1"
base_dir="${RGS_VERIFY_EVIDENCE_DIR:-artifacts/verify}"
run_dir="${base_dir}/${ts}"
proto_mode="${RGS_VERIFY_EVIDENCE_PROTO_MODE:-full}"
require_clean="${RGS_VERIFY_EVIDENCE_REQUIRE_CLEAN:-false}"
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
git_changed_files_count_before="$(git status --porcelain | wc -l | tr -d ' ')"
git_worktree_clean_before="true"
if [[ "${git_changed_files_count_before}" != "0" ]]; then
  git_worktree_clean_before="false"
fi

if [[ "${GITHUB_ACTIONS:-}" == "true" && "${proto_mode}" != "full" ]]; then
  echo "RGS_VERIFY_EVIDENCE_PROTO_MODE must be 'full' in CI (GITHUB_ACTIONS=true), got '${proto_mode}'" >&2
  exit 1
fi

if [[ "${GITHUB_ACTIONS:-}" == "true" && "${require_clean}" != "true" ]]; then
  echo "RGS_VERIFY_EVIDENCE_REQUIRE_CLEAN must be 'true' in CI (GITHUB_ACTIONS=true), got '${require_clean}'" >&2
  exit 1
fi

if [[ "${require_clean}" != "true" && "${require_clean}" != "false" ]]; then
  echo "RGS_VERIFY_EVIDENCE_REQUIRE_CLEAN must be 'true' or 'false', got '${require_clean}'" >&2
  exit 1
fi

if [[ "${require_clean}" == "true" && "${git_worktree_clean_before}" != "true" ]]; then
  echo "verify evidence requires a clean worktree, but detected ${git_changed_files_count_before} changed file(s)" >&2
  git status --short >&2
  exit 1
fi

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
changed_files_file="${run_dir}/changed_files.txt"
index_file="${run_dir}/index.txt"
latest_file="${base_dir}/LATEST"

proto_cmd="RGS_PROTO_CHECK_MODE=${proto_mode} make proto-check"
verify_cmd="RGS_PROTO_CHECK_MODE=${proto_mode} make verify"

proto_status=0
verify_status=0
proto_started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
proto_start_epoch="$(date -u +%s)"

set +e
run_and_capture "${proto_cmd}" "${proto_log}"
proto_status=$?
proto_end_epoch="$(date -u +%s)"
proto_finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
proto_duration_seconds=$((proto_end_epoch - proto_start_epoch))

verify_started_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
verify_start_epoch="$(date -u +%s)"
run_and_capture "${verify_cmd}" "${verify_log}"
verify_status=$?
verify_end_epoch="$(date -u +%s)"
verify_finished_at="$(date -u +%Y-%m-%dT%H:%M:%SZ)"
verify_duration_seconds=$((verify_end_epoch - verify_start_epoch))
set -e

failed_step="null"
if [[ ${proto_status} -ne 0 && ${verify_status} -ne 0 ]]; then
  failed_step="\"both\""
elif [[ ${proto_status} -ne 0 ]]; then
  failed_step="\"proto_check\""
elif [[ ${verify_status} -ne 0 ]]; then
  failed_step="\"make_verify\""
fi

git_changed_files_count_after="$(git status --porcelain | wc -l | tr -d ' ')"
git_worktree_clean_after="true"
if [[ "${git_changed_files_count_after}" != "0" ]]; then
  git_worktree_clean_after="false"
fi

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

go_mod_sha256=""
go_sum_sha256=""
check_module_path_sha256=""
check_proto_clean_sha256=""
verify_evidence_sha256=""
makefile_sha256=""
ci_workflow_sha256=""
if [[ -f "go.mod" ]]; then
  go_mod_sha256="$(checksum_file "go.mod" | awk '{print $1}')"
fi
if [[ -f "go.sum" ]]; then
  go_sum_sha256="$(checksum_file "go.sum" | awk '{print $1}')"
fi
if [[ -f "scripts/check_module_path.sh" ]]; then
  check_module_path_sha256="$(checksum_file "scripts/check_module_path.sh" | awk '{print $1}')"
fi
if [[ -f "scripts/check_proto_clean.sh" ]]; then
  check_proto_clean_sha256="$(checksum_file "scripts/check_proto_clean.sh" | awk '{print $1}')"
fi
if [[ -f "scripts/verify_evidence.sh" ]]; then
  verify_evidence_sha256="$(checksum_file "scripts/verify_evidence.sh" | awk '{print $1}')"
fi
if [[ -f "Makefile" ]]; then
  makefile_sha256="$(checksum_file "Makefile" | awk '{print $1}')"
fi
if [[ -f ".github/workflows/ci.yml" ]]; then
  ci_workflow_sha256="$(checksum_file ".github/workflows/ci.yml" | awk '{print $1}')"
fi

cat >"${summary_file}" <<EOF
{
  "summary_schema_version": ${summary_schema_version},
  "timestamp_utc": "${ts}",
  "git_commit": "${git_commit}",
  "git_branch": "${git_branch}",
  "git_worktree_clean_before": ${git_worktree_clean_before},
  "git_changed_files_count_before": ${git_changed_files_count_before},
  "git_worktree_clean_after": ${git_worktree_clean_after},
  "git_changed_files_count_after": ${git_changed_files_count_after},
  "git_worktree_clean": ${git_worktree_clean_after},
  "git_changed_files_count": ${git_changed_files_count_after},
  "proto_mode": "${proto_mode}",
  "require_clean_worktree": ${require_clean},
  "github_actions": $([[ "${GITHUB_ACTIONS:-}" == "true" ]] && echo "true" || echo "false"),
  "ci_run_id": "${ci_run_id}",
  "ci_run_attempt": "${ci_run_attempt}",
  "ci_ref": "${ci_ref}",
  "ci_sha": "${ci_sha}",
  "hostname": "${host_name}",
  "os": "${os_name}",
  "arch": "${arch_name}",
  "go_version": "${go_version}",
  "buf_version": "${buf_version}",
  "go_mod_sha256": "${go_mod_sha256}",
  "go_sum_sha256": "${go_sum_sha256}",
  "check_module_path_script_sha256": "${check_module_path_sha256}",
  "check_proto_clean_script_sha256": "${check_proto_clean_sha256}",
  "verify_evidence_script_sha256": "${verify_evidence_sha256}",
  "makefile_sha256": "${makefile_sha256}",
  "ci_workflow_sha256": "${ci_workflow_sha256}",
  "proto_check_command": "${proto_cmd}",
  "proto_check_started_at": "${proto_started_at}",
  "proto_check_finished_at": "${proto_finished_at}",
  "proto_check_duration_seconds": ${proto_duration_seconds},
  "make_verify_command": "${verify_cmd}",
  "make_verify_started_at": "${verify_started_at}",
  "make_verify_finished_at": "${verify_finished_at}",
  "make_verify_duration_seconds": ${verify_duration_seconds},
  "proto_check_status": ${proto_status},
  "make_verify_status": ${verify_status},
  "overall_status": $([[ ${proto_status} -eq 0 && ${verify_status} -eq 0 ]] && echo "\"pass\"" || echo "\"fail\""),
  "failed_step": ${failed_step},
  "changed_files_artifact": $([[ "${git_worktree_clean_after}" == "true" ]] && echo "null" || echo "\"changed_files.txt\"")
}
EOF

if [[ "${git_worktree_clean_after}" != "true" ]]; then
  git status --porcelain >"${changed_files_file}"
fi

{
  echo "verify evidence artifact index"
  echo "timestamp_utc=${ts}"
  echo "run_dir=${run_dir}"
  for f in "${proto_log}" "${verify_log}" "${summary_file}" "${changed_files_file}"; do
    if [[ -f "${f}" ]]; then
      # Format: relative_path<TAB>bytes
      rel="${f#${run_dir}/}"
      bytes="$(wc -c <"${f}" | tr -d ' ')"
      printf '%s\t%s\n' "${rel}" "${bytes}"
    fi
  done
} >"${index_file}"

{
  if [[ -f "go.mod" ]]; then
    checksum_file "go.mod" || { echo "no sha256 tool available" >&2; exit 1; }
  fi
  if [[ -f "go.sum" ]]; then
    checksum_file "go.sum" || { echo "no sha256 tool available" >&2; exit 1; }
  fi
  if [[ -f "scripts/check_module_path.sh" ]]; then
    checksum_file "scripts/check_module_path.sh" || { echo "no sha256 tool available" >&2; exit 1; }
  fi
  if [[ -f "scripts/check_proto_clean.sh" ]]; then
    checksum_file "scripts/check_proto_clean.sh" || { echo "no sha256 tool available" >&2; exit 1; }
  fi
  if [[ -f "scripts/verify_evidence.sh" ]]; then
    checksum_file "scripts/verify_evidence.sh" || { echo "no sha256 tool available" >&2; exit 1; }
  fi
  if [[ -f "Makefile" ]]; then
    checksum_file "Makefile" || { echo "no sha256 tool available" >&2; exit 1; }
  fi
  if [[ -f ".github/workflows/ci.yml" ]]; then
    checksum_file ".github/workflows/ci.yml" || { echo "no sha256 tool available" >&2; exit 1; }
  fi
  checksum_file "${proto_log}" || { echo "no sha256 tool available" >&2; exit 1; }
  checksum_file "${verify_log}" || { echo "no sha256 tool available" >&2; exit 1; }
  checksum_file "${summary_file}" || { echo "no sha256 tool available" >&2; exit 1; }
  checksum_file "${index_file}" || { echo "no sha256 tool available" >&2; exit 1; }
  if [[ -f "${changed_files_file}" ]]; then
    checksum_file "${changed_files_file}" || { echo "no sha256 tool available" >&2; exit 1; }
  fi
} >"${manifest_file}"

printf '%s\n' "${run_dir}" >"${latest_file}"

if [[ ${proto_status} -ne 0 || ${verify_status} -ne 0 ]]; then
  echo "verification evidence failed; see ${summary_file}" >&2
  exit 1
fi

echo "verification evidence captured at ${run_dir}"
