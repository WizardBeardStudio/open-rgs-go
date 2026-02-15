# Production Evidence Checklist

Use this checklist to assemble a regulator/operator-ready release packet.

Release gate companion:
- `docs/compliance/GO_LIVE_CHECKLIST.md`

## 1. Build and Release Integrity
- Immutable source revision and signed artifact references.
- `make verify-evidence-strict` results attached for release commit, including:
  - `artifacts/verify/<UTC_TIMESTAMP>/proto_check.log`
  - `artifacts/verify/<UTC_TIMESTAMP>/make_verify.log`
  - `artifacts/verify/<UTC_TIMESTAMP>/summary.json` (includes `summary_schema_version`, verification mode/policy fields, git commit/ref, pre/post worktree cleanliness metadata, CI run metadata, toolchain/runtime context, dependency + verification/build-policy script checksums, command timing metadata, and explicit failure-step attribution)
  - `artifacts/verify/<UTC_TIMESTAMP>/index.txt` (artifact inventory with per-file byte sizes)
  - `artifacts/verify/<UTC_TIMESTAMP>/changed_files.txt` (present when `summary.json` indicates a non-clean worktree)
  - `artifacts/verify/<UTC_TIMESTAMP>/manifest.sha256`
  - `artifacts/verify/LATEST`
- Retain verification artifacts for each release candidate and release for a minimum of 12 months (or longer if jurisdictional policy requires).
- CI artifact naming should include run and attempt identity (for example: `verify-evidence-<run_id>-<run_attempt>`).
- CI evidence capture should enforce a clean worktree (`RGS_VERIFY_EVIDENCE_REQUIRE_CLEAN=true`) to avoid ambiguous provenance.
- `buf lint` and `buf generate` validation evidence.
- Migration plan and target schema version evidence.

## 2. Security Controls Evidence
- TLS/mTLS runtime configuration snapshots.
- JWT keyset configuration and active key rotation procedure.
- `make keyset-evidence` artifact pack (`keyset.json`, `summary.json`, `fingerprint.sha256`) per rotation event.
- Identity lockout policy settings and test results.
- Identity session/admin actor-mismatch denial samples (`RefreshToken`, `Logout`, credential/lockout admin endpoints) with corresponding denied-audit records.
- Remote admin boundary CIDR policy and denied-attempt samples.

## 3. Audit and Immutability Evidence
- Audit append-only hash-chain verification outputs.
- `make audit-chain-evidence` artifact pack (`request_YYYYMMDD.json`, `response_YYYYMMDD.json`, `summary.json`) for API-level chain verification evidence (single or multi-partition run).
- Significant event and alteration retrieval samples.
- Remote access activity retrieval samples (DB-backed mode).
- Change-control evidence for config and download library actions.
- Actor-binding negative-path samples showing `actor mismatch with token` denials and corresponding denied audit events for core service endpoints beyond identity (ledger, wagering, sessions, config, reporting, audit, registry/events, extensions).

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
- `make soak-qual` artifact pack (`benchmark_output.txt`, `summary.json`) for sustained multi-run threshold-gated checks.
- `make soak-qual-db` artifact pack (`benchmark_output.txt`, `summary.json`) for PostgreSQL durability-path sustained checks.
- `make soak-qual-matrix` artifact pack (`matrix_summary.json`, `<profile>/summary.json`) for operator-class profile qualification.

## 6. Compliance Traceability Evidence
- `docs/compliance/REQUIREMENTS.md` with code/test links current to release.
- `docs/compliance/REPORT_CATALOG.md` report definitions and interval coverage.
- `docs/compliance/THREAT_MODEL.md` updated residual risks and mitigations.
- `docs/deployment/` deployment hardening guides.
- Identity actor-binding mismatch denied-audit test artifacts:
  - `internal/platform/server/identity_grpc_test.go`
  - `internal/platform/server/identity_gateway_test.go`
