#!/usr/bin/env bash
set -euo pipefail

old_module_path='github.com/wizardbeard/open-rgs-go'
current='github.com/wizardbeardstudio/open-rgs-go'
expected_go_package="option go_package = \"${current}/gen/rgs/v1;rgsv1\";"

if command -v rg >/dev/null 2>&1; then
	find_old_module_path_refs() {
		rg -n "${old_module_path}" . --glob '!scripts/check_module_path.sh'
	}
	find_proto_mismatch() {
		rg -n '^option go_package = ' api/proto/rgs/v1/*.proto | rg -vF "${expected_go_package}"
	}
else
	find_old_module_path_refs() {
		grep -Rns --exclude='check_module_path.sh' --exclude-dir='.git' -- "${old_module_path}" .
	}
	find_proto_mismatch() {
		grep -n '^option go_package = ' api/proto/rgs/v1/*.proto | grep -vF "${expected_go_package}"
	}
fi

if [[ "$(go list -m)" != "${current}" ]]; then
	echo "module path mismatch: expected ${current}, got $(go list -m)" >&2
	exit 1
fi

if find_old_module_path_refs >/dev/null; then
	echo "old module path references detected: ${old_module_path}" >&2
	find_old_module_path_refs
	exit 1
fi

if find_proto_mismatch >/dev/null; then
	echo "proto go_package mismatch: expected ${expected_go_package}" >&2
	find_proto_mismatch
	exit 1
fi

echo "module path check passed: ${current}"
