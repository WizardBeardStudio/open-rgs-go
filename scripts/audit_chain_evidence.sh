#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${RGS_AUDIT_VERIFY_URL:-}" ]]; then
  RGS_AUDIT_VERIFY_URL="http://127.0.0.1:8080/v1/audit/chain:verify"
fi
if [[ -z "${RGS_AUDIT_BEARER_TOKEN:-}" ]]; then
  echo "RGS_AUDIT_BEARER_TOKEN is required" >&2
  exit 1
fi
if ! command -v jq >/dev/null 2>&1; then
  echo "jq is required for audit-chain evidence extraction" >&2
  exit 1
fi
if ! command -v curl >/dev/null 2>&1; then
  echo "curl is required for audit-chain evidence capture" >&2
  exit 1
fi

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
event_id="${RGS_AUDIT_CHAIN_EVENT_ID:-audit-chain-${timestamp}}"
root_dir="${RGS_AUDIT_CHAIN_WORKDIR:-/tmp/open-rgs-go-audit-chain}"
out_dir="${root_dir}/${event_id}"
mkdir -p "${out_dir}"

partition_day="${RGS_AUDIT_PARTITION_DAY:-$(date -u +%Y-%m-%d)}"
operator_id="${RGS_AUDIT_OPERATOR_ID:-op-evidence}"

request_file="${out_dir}/request.json"
response_file="${out_dir}/response.json"
summary_file="${out_dir}/summary.json"

cat >"${request_file}" <<EOF_REQ
{
  "meta": {
    "requestId": "${event_id}",
    "actor": {
      "actorId": "${operator_id}",
      "actorType": "ACTOR_TYPE_OPERATOR"
    }
  },
  "partitionDay": "${partition_day}"
}
EOF_REQ

curl --silent --show-error --fail \
  -H "Authorization: Bearer ${RGS_AUDIT_BEARER_TOKEN}" \
  -H "Content-Type: application/json" \
  -X POST "${RGS_AUDIT_VERIFY_URL}" \
  --data @"${request_file}" >"${response_file}"

result_code="$(jq -r '.meta.resultCode // "RESULT_CODE_UNSPECIFIED"' "${response_file}")"
valid="$(jq -r '.valid // false' "${response_file}")"
status="pass"
if [[ "${result_code}" != "RESULT_CODE_OK" || "${valid}" != "true" ]]; then
  status="fail"
fi

cat >"${summary_file}" <<EOF_SUMMARY
{
  "event_id": "${event_id}",
  "captured_at_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "verify_url": "${RGS_AUDIT_VERIFY_URL}",
  "partition_day": "${partition_day}",
  "operator_id": "${operator_id}",
  "result_code": "${result_code}",
  "valid": ${valid},
  "result": "${status}"
}
EOF_SUMMARY

cat <<EOF_OUT
audit-chain evidence created:
  ${request_file}
  ${response_file}
  ${summary_file}
EOF_OUT

if [[ "${status}" != "pass" ]]; then
  exit 1
fi
