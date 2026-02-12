# Deployment Guide: Failover and Partition Drill Evidence

This guide defines a repeatable artifact workflow for failover/partition drills with explicit RTO/RPO capture.

## Inputs Required

- `RGS_FAILOVER_OUTAGE_START_UNIX`:
  Unix second when service outage began.
- `RGS_FAILOVER_RECOVERY_UNIX`:
  Unix second when service recovered and passed health checks.
- `RGS_FAILOVER_LAST_DURABLE_UNIX`:
  Unix second of last confirmed durable write before outage.

Optional threshold gates:

- `RGS_FAILOVER_RTO_MAX_SECONDS`
- `RGS_FAILOVER_RPO_MAX_SECONDS`

Optional metadata:

- `RGS_FAILOVER_EVENT_ID` (default timestamp-based id)
- `RGS_FAILOVER_WORKDIR` (default `/tmp/open-rgs-go-failover`)

## Runbook

From `src/`:

```bash
RGS_FAILOVER_EVENT_ID=drill-20260212-a \
RGS_FAILOVER_OUTAGE_START_UNIX=1739323200 \
RGS_FAILOVER_RECOVERY_UNIX=1739323245 \
RGS_FAILOVER_LAST_DURABLE_UNIX=1739323190 \
RGS_FAILOVER_RTO_MAX_SECONDS=90 \
RGS_FAILOVER_RPO_MAX_SECONDS=30 \
./scripts/failover_evidence_snapshot.sh
```

Or with make:

```bash
RGS_FAILOVER_OUTAGE_START_UNIX=1739323200 \
RGS_FAILOVER_RECOVERY_UNIX=1739323245 \
RGS_FAILOVER_LAST_DURABLE_UNIX=1739323190 \
RGS_FAILOVER_RTO_MAX_SECONDS=90 \
RGS_FAILOVER_RPO_MAX_SECONDS=30 \
make failover-evidence
```

## Artifact

- `${RGS_FAILOVER_WORKDIR:-/tmp/open-rgs-go-failover}/<event_id>/snapshot.json`

The snapshot contains:
- event id
- UTC capture timestamp
- outage/recovery/last-durable epochs
- computed `rto_seconds` and `rpo_seconds`
- threshold values (if provided)
- pass/fail result

Attach this snapshot to the production evidence packet for each drill execution.
