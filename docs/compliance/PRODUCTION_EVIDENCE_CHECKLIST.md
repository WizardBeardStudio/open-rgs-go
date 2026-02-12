# Production Evidence Checklist

Use this checklist to assemble a regulator/operator-ready release packet.

## 1. Build and Release Integrity
- Immutable source revision and signed artifact references.
- `go test ./...` results attached for release commit.
- `buf lint` and `buf generate` validation evidence.
- Migration plan and target schema version evidence.

## 2. Security Controls Evidence
- TLS/mTLS runtime configuration snapshots.
- JWT keyset configuration and active key rotation procedure.
- `make keyset-evidence` artifact pack (`keyset.json`, `summary.json`, `fingerprint.sha256`) per rotation event.
- Identity lockout policy settings and test results.
- Remote admin boundary CIDR policy and denied-attempt samples.

## 3. Audit and Immutability Evidence
- Audit append-only hash-chain verification outputs.
- Significant event and alteration retrieval samples.
- Remote access activity retrieval samples (DB-backed mode).
- Change-control evidence for config and download library actions.

## 4. Financial and Wagering Evidence
- Ledger invariants/property test outputs.
- Idempotency replay tests for financial operations.
- Wager lifecycle test outputs (place, settle, cancel, idempotency).
- Account transaction statement report output sample.

## 5. Operational Resilience Evidence
- Chaos test outputs (communication loss, fail-closed paths).
- Backup/restore drill runbook and last successful drill date.
- `make dr-drill` artifact pack (`open_rgs_go.backup`, `manifest.txt`, `critical_table_counts.csv`, `restore_status.txt`).
- DB failover/partition scenario validation report.
- `make failover-evidence` snapshot (`snapshot.json`) with RTO/RPO thresholds and pass/fail result.
- Alerting rules and dashboard screenshots for key risk indicators.
- `make perf-qual` artifact pack (`benchmark_output.txt`, `summary.txt`) with accepted threshold references.

## 6. Compliance Traceability Evidence
- `docs/compliance/REQUIREMENTS.md` with code/test links current to release.
- `docs/compliance/REPORT_CATALOG.md` report definitions and interval coverage.
- `docs/compliance/THREAT_MODEL.md` updated residual risks and mitigations.
- `docs/deployment/` deployment hardening guides.
