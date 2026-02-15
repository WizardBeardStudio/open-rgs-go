#!/usr/bin/env bash
set -euo pipefail

legacy='github.com/wizardbeard/open-rgs-go'
current='github.com/wizardbeardstudio/open-rgs-go'
expected_go_package="option go_package = \"${current}/gen/rgs/v1;rgsv1\";"

if command -v rg >/dev/null 2>&1; then
	find_legacy() {
		rg -n "${legacy}" . --glob '!scripts/check_module_path.sh'
	}
	find_proto_mismatch() {
		rg -n '^option go_package = ' api/proto/rgs/v1/*.proto | rg -vF "${expected_go_package}"
	}
else
	find_legacy() {
		grep -Rns --exclude='check_module_path.sh' --exclude-dir='.git' -- "${legacy}" .
	}
	find_proto_mismatch() {
		grep -n '^option go_package = ' api/proto/rgs/v1/*.proto | grep -vF "${expected_go_package}"
	}
fi

if [[ "$(go list -m)" != "${current}" ]]; then
	echo "module path mismatch: expected ${current}, got $(go list -m)" >&2
	exit 1
fi

if find_legacy >/dev/null; then
	echo "legacy module path references detected: ${legacy}" >&2
	find_legacy
	exit 1
fi

if find_proto_mismatch >/dev/null; then
	echo "proto go_package mismatch: expected ${expected_go_package}" >&2
	find_proto_mismatch
	exit 1
fi

echo "module path check passed: ${current}"
