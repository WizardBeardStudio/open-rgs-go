#!/usr/bin/env bash
set -euo pipefail

# Generates auditable keyset-rotation evidence from file or command source.
# Input must be a JSON payload:
# {"active_kid":"k2","keys":{"k1":"...","k2":"..."}}

source_mode=""
payload=""
if [[ -n "${RGS_JWT_KEYSET_FILE:-}" ]]; then
  source_mode="file"
  if [[ ! -f "${RGS_JWT_KEYSET_FILE}" ]]; then
    echo "RGS_JWT_KEYSET_FILE not found: ${RGS_JWT_KEYSET_FILE}" >&2
    exit 1
  fi
  payload="$(cat "${RGS_JWT_KEYSET_FILE}")"
elif [[ -n "${RGS_JWT_KEYSET_COMMAND:-}" ]]; then
  source_mode="command"
  payload="$(sh -lc "${RGS_JWT_KEYSET_COMMAND}")"
else
  echo "set RGS_JWT_KEYSET_FILE or RGS_JWT_KEYSET_COMMAND" >&2
  exit 1
fi

if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for keyset evidence extraction" >&2
  exit 1
fi

active_kid="$(printf '%s' "${payload}" | jq -r '.active_kid // "default"')"
keys_count="$(printf '%s' "${payload}" | jq -r '.keys | length')"
if [[ "${keys_count}" == "0" || "${keys_count}" == "null" ]]; then
  echo "keyset payload contains no keys" >&2
  exit 1
fi

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
event_id="${RGS_KEYSET_EVENT_ID:-keyset-${timestamp}}"
root_dir="${RGS_KEYSET_WORKDIR:-/tmp/open-rgs-go-keyset}"
out_dir="${root_dir}/${event_id}"
mkdir -p "${out_dir}"

payload_file="${out_dir}/keyset.json"
summary_file="${out_dir}/summary.json"
fingerprint_file="${out_dir}/fingerprint.sha256"

printf '%s\n' "${payload}" >"${payload_file}"
sha256sum "${payload_file}" >"${fingerprint_file}"

rotation_state="initial"
if [[ -n "${RGS_KEYSET_PREVIOUS_SUMMARY_FILE:-}" && -f "${RGS_KEYSET_PREVIOUS_SUMMARY_FILE}" ]]; then
  prev_active="$(jq -r '.active_kid // "default"' "${RGS_KEYSET_PREVIOUS_SUMMARY_FILE}")"
  if [[ "${prev_active}" != "${active_kid}" ]]; then
    rotation_state="rotated"
  else
    rotation_state="unchanged"
  fi
fi

cat >"${summary_file}" <<EOF
{
  "event_id": "${event_id}",
  "captured_at_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "source_mode": "${source_mode}",
  "active_kid": "${active_kid}",
  "keys_count": ${keys_count},
  "rotation_state": "${rotation_state}"
}
EOF

cat <<EOF
keyset rotation evidence created:
  ${payload_file}
  ${summary_file}
  ${fingerprint_file}
EOF

