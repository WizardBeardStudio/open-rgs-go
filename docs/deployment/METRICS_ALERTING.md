# Deployment Guide: Metrics and Alerting (Phase 5)

This guide defines baseline Prometheus alerting for ledger idempotency retention and cleanup health.

## Metrics Exposed

From `GET /metrics`:

- `open_rgs_ledger_idempotency_cleanup_runs_total{result}`
- `open_rgs_ledger_idempotency_cleanup_deleted_total`
- `open_rgs_ledger_idempotency_cleanup_last_deleted`
- `open_rgs_ledger_idempotency_cleanup_last_run_unix`
- `open_rgs_ledger_idempotency_keys_total`
- `open_rgs_ledger_idempotency_keys_expired`
- `open_rgs_identity_login_attempts_total{result,actor_type}`
- `open_rgs_identity_lockout_activations_total{actor_type}`
- `open_rgs_identity_sessions_active`
- `open_rgs_identity_sessions_revoked`
- `open_rgs_identity_sessions_expired`

## Recommended Baseline Alerts

### 1) Cleanup worker failures

Trigger when cleanup has recent errors:

```promql
increase(open_rgs_ledger_idempotency_cleanup_runs_total{result="error"}[15m]) > 0
```

Suggested severity: `warning` (raise to `critical` if sustained >1h).

### 2) Cleanup worker stalled

Trigger when no cleanup run has been recorded recently:

```promql
time() - open_rgs_ledger_idempotency_cleanup_last_run_unix > 1800
```

This assumes the default 15-minute cleanup interval; adjust threshold to ~2x interval.

Suggested severity: `critical`.

### 3) Expired-key backlog growth

Trigger when expired keys are accumulating faster than cleanup:

```promql
open_rgs_ledger_idempotency_keys_expired > 1000
```

Tune threshold by traffic profile and storage budget.

Suggested severity: `warning` (raise to `critical` at higher threshold, e.g. `> 10000`).

### 4) Total key volume anomaly

Trigger when total idempotency keys exceed expected capacity:

```promql
open_rgs_ledger_idempotency_keys_total > 500000
```

Tune threshold by expected write rate and retention TTL.

Suggested severity: `warning`.

### 5) Identity login denial-rate spike

Trigger when denied+invalid logins are too high relative to successful logins:

```promql
sum(increase(open_rgs_identity_login_attempts_total{result=~"denied|invalid"}[15m]))
/
clamp_min(sum(increase(open_rgs_identity_login_attempts_total{result="ok"}[15m])), 1)
> 3
```

Suggested severity: `warning` (raise to `critical` for sustained spikes).

### 6) Identity lockout activation surge

Trigger when lockouts are being activated frequently:

```promql
sum(increase(open_rgs_identity_lockout_activations_total[15m])) > 10
```

Suggested severity: `critical`.

### 7) Identity expired-session backlog

Trigger when expired sessions are not being cleaned up:

```promql
open_rgs_identity_sessions_expired > 5000
```

Suggested severity: `warning`.

## Operational Tuning Notes

- If `open_rgs_ledger_idempotency_keys_expired` remains high:
  - lower `RGS_LEDGER_IDEMPOTENCY_TTL` if policy allows
  - decrease `RGS_LEDGER_IDEMPOTENCY_CLEANUP_INTERVAL`
  - increase `RGS_LEDGER_IDEMPOTENCY_CLEANUP_BATCH`
- Keep dashboards for:
  - cleanup run outcomes (`success` vs `error`)
  - expired keys gauge
  - total keys gauge
  - last cleanup timestamp
  - identity login outcomes (`ok` / `denied` / `invalid` / `error`)
  - lockout activation rate
  - active/revoked/expired session gauges

## Rule Group Example (YAML)

```yaml
groups:
  - name: open-rgs-ledger-idempotency
    rules:
      - alert: OpenRGSIdempotencyCleanupErrors
        expr: increase(open_rgs_ledger_idempotency_cleanup_runs_total{result="error"}[15m]) > 0
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "open-rgs idempotency cleanup errors detected"
          description: "Cleanup worker has reported one or more errors in the last 15 minutes."

      - alert: OpenRGSIdempotencyCleanupStalled
        expr: time() - open_rgs_ledger_idempotency_cleanup_last_run_unix > 1800
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "open-rgs idempotency cleanup appears stalled"
          description: "No idempotency cleanup run has been observed in the last 30 minutes."

      - alert: OpenRGSIdempotencyExpiredBacklog
        expr: open_rgs_ledger_idempotency_keys_expired > 1000
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "open-rgs expired idempotency key backlog is high"
          description: "Expired idempotency keys exceed threshold."

  - name: open-rgs-identity-auth
    rules:
      - alert: OpenRGSIdentityDeniedLoginSpike
        expr: sum(increase(open_rgs_identity_login_attempts_total{result=~"denied|invalid"}[15m])) / clamp_min(sum(increase(open_rgs_identity_login_attempts_total{result="ok"}[15m])), 1) > 3
        for: 10m
        labels:
          severity: warning
        annotations:
          summary: "open-rgs identity denied login ratio is elevated"
          description: "Denied/invalid logins are significantly higher than successful logins."

      - alert: OpenRGSIdentityLockoutSurge
        expr: sum(increase(open_rgs_identity_lockout_activations_total[15m])) > 10
        for: 10m
        labels:
          severity: critical
        annotations:
          summary: "open-rgs identity lockout activations are surging"
          description: "Lockout activations exceeded expected threshold in the last 15 minutes."

      - alert: OpenRGSIdentityExpiredSessionsBacklog
        expr: open_rgs_identity_sessions_expired > 5000
        for: 15m
        labels:
          severity: warning
        annotations:
          summary: "open-rgs identity expired session backlog is high"
          description: "Expired identity sessions exceed threshold; cleanup may be lagging."
```
