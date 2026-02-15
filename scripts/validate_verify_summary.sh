#!/usr/bin/env bash
set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: ./scripts/validate_verify_summary.sh <summary.json>" >&2
  exit 2
fi

summary_file="$1"

if [[ ! -f "${summary_file}" ]]; then
  echo "summary file not found: ${summary_file}" >&2
  exit 1
fi

go run ./cmd/verifysummary "${summary_file}"
