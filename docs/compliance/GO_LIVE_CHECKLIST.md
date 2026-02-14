# Go-Live Checklist

Use this checklist as the release gate for production launch readiness.

Release metadata:
- Release version:
- Target environment:
- Target jurisdiction(s):
- Release manager:
- Planned go-live date (UTC):

## Gate 1: Environment Hardening Complete
- Owner: Platform/SRE
- Status: `PASS` / `FAIL`
- Criteria:
  - TLS/mTLS enabled and cert chain validated
  - Trusted CIDRs configured for admin boundary
  - PostgreSQL-backed production mode enabled
  - `RGS_STRICT_PRODUCTION_MODE=true`
  - `RGS_STRICT_EXTERNAL_JWT_KEYSET=true`
  - No default credentials/secrets
- Evidence:
  - Runtime config snapshot
  - `docs/deployment/FIREWALL_LOGGING.md` control validation notes

## Gate 2: External Key Custody Operationalized
- Owner: Security/Platform
- Status: `PASS` / `FAIL`
- Criteria:
  - `RGS_JWT_KEYSET_FILE` or `RGS_JWT_KEYSET_COMMAND` configured in production
  - Key rotation procedure executed in current cycle
  - Strict external keyset enforcement confirmed
- Evidence:
  - `make keyset-evidence` artifacts:
    - `keyset.json`
    - `summary.json`
    - `fingerprint.sha256`
  - `docs/deployment/KEY_MANAGEMENT.md` runbook execution notes

## Gate 3: Data Durability and Recovery Proven
- Owner: DB/SRE
- Status: `PASS` / `FAIL`
- Criteria:
  - Backup artifact successfully created
  - Restore drill completed in isolated target
  - Data integrity spot checks passed post-restore
  - Audit chain verification API run passes for selected partition day
- Evidence:
  - `make dr-drill` artifacts:
    - `open_rgs_go.backup`
    - `manifest.txt`
    - `critical_table_counts.csv`
    - `restore_status.txt`
  - `make audit-chain-evidence` artifacts:
    - `request_YYYYMMDD.json`
    - `response_YYYYMMDD.json`
    - `summary.json`
  - `docs/deployment/DR_DRILLS.md` execution record
  - `docs/deployment/AUDIT_CHAIN_VERIFICATION.md` execution record

## Gate 4: Failover and Partition Recovery Proven
- Owner: Platform/SRE
- Status: `PASS` / `FAIL`
- Criteria:
  - Drill event recorded
  - RTO within target
  - RPO within target
- Evidence:
  - `make failover-evidence` artifact:
    - `snapshot.json`
  - `docs/deployment/FAILOVER_DRILLS.md` execution record

## Gate 5: Performance Qualification Signed Off
- Owner: Performance/Platform
- Status: `PASS` / `FAIL`
- Criteria:
  - Baseline benchmark thresholds met
  - Sustained soak thresholds met
  - No regression vs prior accepted release
- Evidence:
  - `make perf-qual` artifacts:
    - `benchmark_output.txt`
    - `summary.txt`
  - `make soak-qual` artifacts:
    - `benchmark_output.txt`
    - `summary.json`
  - `make soak-qual-db` artifacts:
    - `benchmark_output.txt`
    - `summary.json`
  - `make soak-qual-matrix` artifacts:
    - `matrix_summary.json`
    - `<profile>/summary.json`
  - `docs/deployment/PERFORMANCE_QUALIFICATION.md`
  - `docs/deployment/LOAD_SOAK_QUALIFICATION.md`

## Gate 6: Security Monitoring and Alerting Live
- Owner: SRE/SecOps
- Status: `PASS` / `FAIL`
- Criteria:
  - Alert rules deployed
  - Alerts routed to on-call
  - Test alerts acknowledged and resolved through runbook
- Evidence:
  - Alert manager policy snapshot
  - Dashboard captures
  - `docs/deployment/METRICS_ALERTING.md` validation notes

## Gate 7: Compliance Evidence Packet Complete
- Owner: Compliance/QA
- Status: `PASS` / `FAIL`
- Criteria:
  - All sections complete in evidence checklist
  - Requirements traceability updated to release commit
  - Threat model reviewed for residual risks
  - gRPC/REST parity negative-path evidence includes actor mismatch with token denial coverage for core state and reporting/admin surfaces
- Evidence:
  - `docs/compliance/PRODUCTION_EVIDENCE_CHECKLIST.md`
  - `docs/compliance/REQUIREMENTS.md`
  - `docs/compliance/THREAT_MODEL.md`
  - `docs/compliance/REPORT_CATALOG.md`
  - `internal/platform/server/extensions_grpc_test.go`
  - `internal/platform/server/extensions_gateway_test.go`
  - `internal/platform/server/identity_grpc_test.go`
  - `internal/platform/server/identity_gateway_test.go`
  - `internal/platform/server/ledger_grpc_test.go`
  - `internal/platform/server/ledger_gateway_test.go`
  - `internal/platform/server/wagering_grpc_test.go`
  - `internal/platform/server/wagering_gateway_test.go`
  - `internal/platform/server/sessions_grpc_test.go`
  - `internal/platform/server/sessions_gateway_test.go`
  - `internal/platform/server/config_grpc_test.go`
  - `internal/platform/server/config_gateway_test.go`
  - `internal/platform/server/reporting_grpc_test.go`
  - `internal/platform/server/reporting_gateway_test.go`
  - `internal/platform/server/audit_grpc_test.go`
  - `internal/platform/server/registry_events_test.go`
  - `internal/platform/server/registry_events_gateway_test.go`

## Gate 8: Domain Scope Acceptance
- Owner: Product + Compliance
- Status: `PASS` / `FAIL`
- Criteria:
  - Advanced promotions/UI gap either:
    - implemented and validated, or
    - formally accepted as deferred scope with approvals
- Evidence:
  - Signed scope-acceptance memo or implementation test report

## Gate 9: Jurisdiction and Lab Submission Readiness
- Owner: Compliance/Regulatory
- Status: `PASS` / `FAIL`
- Criteria:
  - Submission packet assembled
  - Jurisdiction-specific controls mapped
  - Internal pre-submission review completed
- Evidence:
  - Submission manifest
  - Review sign-off record

## Final Release Decision
- Go-live decision: `APPROVED` / `REJECTED`
- Decision date (UTC):
- Decision authority:
- Notes:

Sign-offs:
- Engineering lead:
- Platform/SRE lead:
- Security lead:
- Compliance lead:
- Product owner:
