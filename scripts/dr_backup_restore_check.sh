#!/usr/bin/env bash
set -euo pipefail

# Automates a production-evidence DR drill artifact pack:
# 1) creates a PostgreSQL backup snapshot
# 2) records critical-table row counts
# 3) optionally restores into a dedicated drill database URL

if [[ -z "${RGS_DATABASE_URL:-}" ]]; then
  echo "RGS_DATABASE_URL is required" >&2
  exit 1
fi

for bin in pg_dump psql sha256sum; do
  if ! command -v "${bin}" >/dev/null 2>&1; then
    echo "required command not found: ${bin}" >&2
    exit 1
  fi
done

timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
root_dir="${RGS_DRILL_WORKDIR:-/tmp/open-rgs-go-dr}"
out_dir="${root_dir}/${timestamp}"
mkdir -p "${out_dir}"

backup_file="${out_dir}/open_rgs_go.backup"
manifest_file="${out_dir}/manifest.txt"
counts_file="${out_dir}/critical_table_counts.csv"

echo "creating backup at ${backup_file}"
pg_dump --format=custom --file="${backup_file}" "${RGS_DATABASE_URL}"

echo "hashing backup"
sha256sum "${backup_file}" >"${manifest_file}"

echo "capturing critical table counts"
psql "${RGS_DATABASE_URL}" -v ON_ERROR_STOP=1 -At <<'SQL' >"${counts_file}"
SELECT table_name || ',' || row_estimate
FROM (
  SELECT 'audit_events' AS table_name, COALESCE(reltuples::bigint, 0) AS row_estimate
  FROM pg_class WHERE relname = 'audit_events'
  UNION ALL
  SELECT 'ledger_entries', COALESCE(reltuples::bigint, 0)
  FROM pg_class WHERE relname = 'ledger_entries'
  UNION ALL
  SELECT 'ledger_transactions', COALESCE(reltuples::bigint, 0)
  FROM pg_class WHERE relname = 'ledger_transactions'
  UNION ALL
  SELECT 'wagers', COALESCE(reltuples::bigint, 0)
  FROM pg_class WHERE relname = 'wagers'
  UNION ALL
  SELECT 'significant_events', COALESCE(reltuples::bigint, 0)
  FROM pg_class WHERE relname = 'significant_events'
  UNION ALL
  SELECT 'meter_snapshots', COALESCE(reltuples::bigint, 0)
  FROM pg_class WHERE relname = 'meter_snapshots'
  UNION ALL
  SELECT 'meter_deltas', COALESCE(reltuples::bigint, 0)
  FROM pg_class WHERE relname = 'meter_deltas'
  UNION ALL
  SELECT 'remote_access_activity', COALESCE(reltuples::bigint, 0)
  FROM pg_class WHERE relname = 'remote_access_activity'
) t
ORDER BY table_name;
SQL

if [[ -n "${RGS_DRILL_RESTORE_URL:-}" ]]; then
  if ! command -v pg_restore >/dev/null 2>&1; then
    echo "required command not found: pg_restore" >&2
    exit 1
  fi
  echo "restoring backup into drill target DB"
  pg_restore --clean --if-exists --no-owner --no-privileges --dbname="${RGS_DRILL_RESTORE_URL}" "${backup_file}"
  echo "restore complete for ${RGS_DRILL_RESTORE_URL}" >"${out_dir}/restore_status.txt"
else
  echo "restore skipped (set RGS_DRILL_RESTORE_URL to run restore verification)" >"${out_dir}/restore_status.txt"
fi

cat <<EOF
drill artifacts created:
  ${backup_file}
  ${manifest_file}
  ${counts_file}
  ${out_dir}/restore_status.txt
EOF
