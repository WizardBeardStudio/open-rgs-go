#!/usr/bin/env bash
set -euo pipefail

if [[ -z "${RGS_FAILOVER_OUTAGE_START_UNIX:-}" ]]; then
  echo "RGS_FAILOVER_OUTAGE_START_UNIX is required" >&2
  exit 1
fi
if [[ -z "${RGS_FAILOVER_RECOVERY_UNIX:-}" ]]; then
  echo "RGS_FAILOVER_RECOVERY_UNIX is required" >&2
  exit 1
fi
if [[ -z "${RGS_FAILOVER_LAST_DURABLE_UNIX:-}" ]]; then
  echo "RGS_FAILOVER_LAST_DURABLE_UNIX is required" >&2
  exit 1
fi

event_id="${RGS_FAILOVER_EVENT_ID:-failover-$(date -u +%Y%m%dT%H%M%SZ)}"
root_dir="${RGS_FAILOVER_WORKDIR:-/tmp/open-rgs-go-failover}"
out_dir="${root_dir}/${event_id}"
mkdir -p "${out_dir}"

outage_start="${RGS_FAILOVER_OUTAGE_START_UNIX}"
recovery_at="${RGS_FAILOVER_RECOVERY_UNIX}"
last_durable="${RGS_FAILOVER_LAST_DURABLE_UNIX}"

rto_seconds=$(( recovery_at - outage_start ))
rpo_seconds=$(( outage_start - last_durable ))
if (( rto_seconds < 0 || rpo_seconds < 0 )); then
  echo "computed negative duration (check unix timestamps)" >&2
  exit 1
fi

status="pass"
if [[ -n "${RGS_FAILOVER_RTO_MAX_SECONDS:-}" ]] && (( rto_seconds > RGS_FAILOVER_RTO_MAX_SECONDS )); then
  status="fail"
fi
if [[ -n "${RGS_FAILOVER_RPO_MAX_SECONDS:-}" ]] && (( rpo_seconds > RGS_FAILOVER_RPO_MAX_SECONDS )); then
  status="fail"
fi

snapshot_file="${out_dir}/snapshot.json"
cat >"${snapshot_file}" <<EOF
{
  "event_id": "${event_id}",
  "captured_at_utc": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
  "outage_start_unix": ${outage_start},
  "recovery_unix": ${recovery_at},
  "last_durable_unix": ${last_durable},
  "rto_seconds": ${rto_seconds},
  "rpo_seconds": ${rpo_seconds},
  "rto_max_seconds": ${RGS_FAILOVER_RTO_MAX_SECONDS:-null},
  "rpo_max_seconds": ${RGS_FAILOVER_RPO_MAX_SECONDS:-null},
  "result": "${status}"
}
EOF

cat <<EOF
failover evidence created:
  ${snapshot_file}
  rto_seconds=${rto_seconds}
  rpo_seconds=${rpo_seconds}
  result=${status}
EOF

if [[ "${status}" != "pass" ]]; then
  exit 1
fi
