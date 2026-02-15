#!/usr/bin/env bash
set -euo pipefail

legacy='github.com/wizardbeard/open-rgs-go'
current='github.com/wizardbeardstudio/open-rgs-go'
expected_go_package="option go_package = \"${current}/gen/rgs/v1;rgsv1\";"

if [[ "$(go list -m)" != "${current}" ]]; then
	echo "module path mismatch: expected ${current}, got $(go list -m)" >&2
	exit 1
fi

if rg -n "${legacy}" . --glob '!scripts/check_module_path.sh' >/dev/null; then
	echo "legacy module path references detected: ${legacy}" >&2
	rg -n "${legacy}" . --glob '!scripts/check_module_path.sh'
	exit 1
fi

if rg -n '^option go_package = ' api/proto/rgs/v1/*.proto | rg -vF "${expected_go_package}" >/dev/null; then
	echo "proto go_package mismatch: expected ${expected_go_package}" >&2
	rg -n '^option go_package = ' api/proto/rgs/v1/*.proto | rg -vF "${expected_go_package}"
	exit 1
fi

echo "module path check passed: ${current}"
