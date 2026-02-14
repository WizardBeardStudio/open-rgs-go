#!/usr/bin/env bash
set -euo pipefail

legacy='github.com/wizardbeard/open-rgs-go'
current='github.com/wizardbeardstudio/open-rgs-go'

if [[ "$(go list -m)" != "${current}" ]]; then
	echo "module path mismatch: expected ${current}, got $(go list -m)" >&2
	exit 1
fi

if rg -n "${legacy}" . --glob '!scripts/check_module_path.sh' >/dev/null; then
	echo "legacy module path references detected: ${legacy}" >&2
	rg -n "${legacy}" . --glob '!scripts/check_module_path.sh'
	exit 1
fi

echo "module path check passed: ${current}"
