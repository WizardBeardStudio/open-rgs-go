#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: ./scripts/validate_verify_summary.sh <summary.json>" >&2
  exit 2
fi

summary_file="$1"
validate_mode="${RGS_VALIDATE_SUMMARY_MODE:-strict}"

if [[ ! -f "${summary_file}" ]]; then
  echo "summary file not found: ${summary_file}" >&2
  exit 1
fi

go run ./cmd/verifysummary "${summary_file}"

if [[ "${validate_mode}" == "json" ]]; then
  exit 0
fi
if [[ "${validate_mode}" != "strict" ]]; then
  echo "invalid RGS_VALIDATE_SUMMARY_MODE: ${validate_mode} (expected strict|json)" >&2
  exit 1
fi

extract_json_string_field() {
  local file="$1"
  local key="$2"
  sed -n "s/.*\"${key}\":[[:space:]]*\"\\([^\"]*\\)\".*/\\1/p" "${file}" | head -n1
}

summary_dir="$(cd "$(dirname "${summary_file}")" && pwd -P)"
validation_log_rel="$(extract_json_string_field "${summary_file}" "summary_validation_log")"
validation_log_sha="$(extract_json_string_field "${summary_file}" "summary_validation_log_sha256")"
run_dir="$(extract_json_string_field "${summary_file}" "run_dir")"

if [[ -z "${validation_log_rel}" || -z "${validation_log_sha}" ]]; then
  echo "summary missing validation log linkage fields" >&2
  exit 1
fi
if [[ "${validation_log_rel}" == *"/"* ]]; then
  echo "summary_validation_log must be a file name, got path: ${validation_log_rel}" >&2
  exit 1
fi
if [[ "${run_dir}" != "$(dirname "${summary_file}")" ]]; then
  echo "run_dir mismatch: summary has '${run_dir}' but file is in '$(dirname "${summary_file}")'" >&2
  exit 1
fi

validation_log_path="${summary_dir}/${validation_log_rel}"
if [[ ! -f "${validation_log_path}" ]]; then
  echo "summary validation log not found: ${validation_log_path}" >&2
  exit 1
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

actual_sha="$(checksum_file "${validation_log_path}" | awk '{print $1}')"
if [[ -z "${actual_sha}" ]]; then
  echo "failed to compute sha256 for ${validation_log_path}" >&2
  exit 1
fi
if [[ "${actual_sha,,}" != "${validation_log_sha,,}" ]]; then
  echo "summary_validation_log_sha256 mismatch: expected=${validation_log_sha} actual=${actual_sha}" >&2
  exit 1
fi
