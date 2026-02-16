#!/usr/bin/env bash
set -euo pipefail

# Compatibility wrapper. The dedicated utility now lives in cmd/attestkeygen.
repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"
exec go run ./cmd/attestkeygen "$@"
