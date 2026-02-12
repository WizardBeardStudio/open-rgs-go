# Deployment Guide: DR Backup/Restore Drills

This guide defines the baseline disaster-recovery evidence workflow for open-rgs-go PostgreSQL deployments.

## Objectives

- Produce auditable backup artifacts.
- Record integrity evidence (hash manifest).
- Capture critical table cardinality snapshots.
- Optionally verify restore into a dedicated drill target database.

## Prerequisites

- `pg_dump`, `psql`, `sha256sum` available in `PATH`
- Optional for restore verification: `pg_restore`
- `RGS_DATABASE_URL` for source backup
- Optional `RGS_DRILL_RESTORE_URL` for drill restore target

## Runbook

From `src/`:

```bash
RGS_DATABASE_URL="postgres://user:pass@localhost:5432/open_rgs_go?sslmode=disable" \
RGS_DRILL_RESTORE_URL="postgres://user:pass@localhost:5432/open_rgs_go_drill?sslmode=disable" \
./scripts/dr_backup_restore_check.sh
```

Or with make:

```bash
RGS_DATABASE_URL="postgres://user:pass@localhost:5432/open_rgs_go?sslmode=disable" \
RGS_DRILL_RESTORE_URL="postgres://user:pass@localhost:5432/open_rgs_go_drill?sslmode=disable" \
make dr-drill
```

## Artifacts Produced

Artifacts are written under `${RGS_DRILL_WORKDIR:-/tmp/open-rgs-go-dr}/<UTC timestamp>/`:

- `open_rgs_go.backup` (custom pg_dump format)
- `manifest.txt` (SHA-256 digest)
- `critical_table_counts.csv` (estimated row counts for critical tables)
- `restore_status.txt` (restore execution status)

## Operational Notes

- Run this drill periodically (recommended at least monthly) and attach artifacts to the release evidence packet.
- Always restore into an isolated drill database, not production.
- Pair this drill with application-level checks (`go test ./...`, smoke APIs, and report-generation checks) after restore.
