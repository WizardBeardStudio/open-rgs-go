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

go run ./cmd/verifysummary --mode="${validate_mode}" "${summary_file}"
