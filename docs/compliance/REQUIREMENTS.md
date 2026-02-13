# Compliance Requirements Traceability

This document maps implemented requirements to standards references, code locations, and automated tests.

## RGS-0001 Protocol Boundary and Authn/Authz
- Standard refs: GLI-13 / GLI-21 (controlled interfaces, remote access controls)
- Code: `cmd/rgsd/main.go`, `internal/platform/auth/jwt.go`, `internal/platform/server/ledger_grpc.go`, `internal/platform/server/registry_grpc.go`, `internal/platform/server/events_grpc.go`
- Tests: `internal/platform/auth/jwt_test.go`, `internal/platform/server/ledger_grpc_test.go`, `internal/platform/server/registry_events_test.go`
- Status: implemented (baseline)

## RGS-0002 Append-only Audit Logging and Integrity Chaining
- Standard refs: GLI-13 significant events and alterations reporting
- Code: `internal/platform/audit/model.go`, `internal/platform/audit/chain.go`, `internal/platform/audit/store.go`, `migrations/000001_init_core.up.sql`
- Tests: `internal/platform/audit/chain_test.go`
- Status: implemented (in-memory + DB schema controls)

## RGS-0003 Deterministic Time Handling for Reports and Eventing
- Standard refs: GLI-13 date/time tracking expectations
- Code: `internal/platform/clock/clock.go`, `internal/platform/server/system_grpc.go`, `internal/platform/server/ledger_grpc.go`, `internal/platform/server/events_grpc.go`
- Tests: `internal/platform/server/system_gateway_test.go`, `internal/platform/server/events_replay_test.go`
- Status: implemented (clock abstraction used by services)

## RGS-0101 Cashless Ledger Semantics and Non-Negative Balances
- Standard refs: GLI-16 cashless transactions and account controls
- Code: `api/proto/rgs/v1/ledger.proto`, `internal/platform/server/ledger_grpc.go`, `migrations/000002_ledger_cashless.up.sql`
- Tests: `internal/platform/server/ledger_grpc_test.go`, `internal/platform/server/ledger_invariants_test.go`
- Status: implemented (in-memory service + schema)

## RGS-0102 Financial Idempotency for Stateful Operations
- Standard refs: GLI-16 financial transaction confirmation/denial and consistency controls
- Code: `api/proto/rgs/v1/common.proto`, `internal/platform/server/ledger_grpc.go`, `migrations/000002_ledger_cashless.up.sql`
- Tests: `internal/platform/server/ledger_grpc_test.go`, `internal/platform/server/ledger_invariants_test.go`
- Status: implemented (request idempotency keys enforced in service)

## RGS-0103 Unresolved Transfer and Partial Transfer Behavior
- Standard refs: GLI-16 transfer failure and partial transfer handling
- Code: `api/proto/rgs/v1/ledger.proto`, `internal/platform/server/ledger_grpc.go`, `migrations/000002_ledger_cashless.up.sql`
- Tests: `internal/platform/server/ledger_grpc_test.go`
- Status: implemented (service behavior + schema scaffold)

## RGS-0104 gRPC and REST Parity (Ledger)
- Standard refs: internal platform requirement (protobuf canonical API mirrored over REST)
- Code: `api/proto/rgs/v1/ledger.proto`, `cmd/rgsd/main.go`
- Tests: `internal/platform/server/ledger_gateway_test.go`
- Status: implemented

## RGS-0201 Equipment Registry (GEAR-like) Model
- Standard refs: GLI-13 registry and equipment lifecycle expectations
- Code: `api/proto/rgs/v1/registry.proto`, `internal/platform/server/registry_grpc.go`, `migrations/000003_registry_events_meters.up.sql`
- Tests: `internal/platform/server/registry_events_test.go`, `internal/platform/server/registry_events_gateway_test.go`
- Status: implemented (DB-backed registry with in-memory fallback; strict production disables in-memory registry mirrors)

## RGS-0202 Significant Event Ingestion with Time Triplet
- Standard refs: GLI-13 significant events and event retention/reporting
- Code: `api/proto/rgs/v1/events.proto`, `internal/platform/server/events_grpc.go`, `migrations/000003_registry_events_meters.up.sql`
- Tests: `internal/platform/server/registry_events_test.go`, `internal/platform/server/events_replay_test.go`
- Status: implemented (DB-backed events with in-memory fallback; strict production disables in-memory events/meters mirrors)

## RGS-0203 Meter Snapshot/Delta Ingestion Semantics
- Standard refs: GLI-13 metering information handling and reporting
- Code: `api/proto/rgs/v1/events.proto`, `internal/platform/server/events_grpc.go`, `migrations/000003_registry_events_meters.up.sql`
- Tests: `internal/platform/server/registry_events_test.go`, `internal/platform/server/events_replay_test.go`
- Status: implemented (in-memory service + schema)

## RGS-0204 Loss Handling and Buffer Exhaustion Disable Behavior
- Standard refs: GLI-13 communication loss/buffering requirements
- Code: `internal/platform/server/events_grpc.go`, `migrations/000003_registry_events_meters.up.sql`
- Tests: `internal/platform/server/registry_events_test.go`
- Status: implemented (buffer queue model with fail-closed disable)

## RGS-0205 gRPC and REST Parity (Registry/Events)
- Standard refs: internal platform requirement (protobuf canonical API mirrored over REST)
- Code: `api/proto/rgs/v1/registry.proto`, `api/proto/rgs/v1/events.proto`, `cmd/rgsd/main.go`
- Tests: `internal/platform/server/registry_events_gateway_test.go`
- Status: implemented

## RGS-0206 Deterministic Replay under Out-of-Order Ingestion
- Standard refs: GLI-aligned deterministic operation and reportability objective
- Code: `internal/platform/server/events_grpc.go`
- Tests: `internal/platform/server/events_replay_test.go`
- Status: implemented

## RGS-0301 On-Demand Reporting with DTD/MTD/YTD/LTD Intervals
- Standard refs: GLI-13 report interval and generation metadata expectations
- Code: `api/proto/rgs/v1/reporting.proto`, `internal/platform/server/reporting_grpc.go`, `migrations/000004_reporting_runs.up.sql`
- Tests: `internal/platform/server/reporting_grpc_test.go`
- Status: implemented (strict production disables in-memory reporting payload fallback and in-memory run retention)

## RGS-0302 Significant Events/Alterations Reporting Content
- Standard refs: GLI-13 significant events and alterations reportability
- Code: `internal/platform/server/reporting_grpc.go`, `internal/platform/server/events_grpc.go`, `api/proto/rgs/v1/reporting.proto`
- Tests: `internal/platform/server/reporting_grpc_test.go`
- Status: implemented

## RGS-0303 Cashless Liability Summary Reporting
- Standard refs: GLI-16 cashless account/transaction reporting expectations
- Code: `internal/platform/server/reporting_grpc.go`, `internal/platform/server/ledger_grpc.go`, `api/proto/rgs/v1/reporting.proto`
- Tests: `internal/platform/server/reporting_grpc_test.go`
- Status: implemented

## RGS-0304 Report Export Formats (JSON and CSV)
- Standard refs: regulator-friendly reporting/export objective
- Code: `internal/platform/server/reporting_grpc.go`, `api/proto/rgs/v1/reporting.proto`
- Tests: `internal/platform/server/reporting_grpc_test.go`, `internal/platform/server/reporting_gateway_test.go`
- Status: implemented

## RGS-0305 gRPC and REST Parity (Reporting)
- Standard refs: internal platform requirement (protobuf canonical API mirrored over REST)
- Code: `api/proto/rgs/v1/reporting.proto`, `cmd/rgsd/main.go`
- Tests: `internal/platform/server/reporting_gateway_test.go`
- Status: implemented

## RGS-0401 Configuration Change Proposal/Approval/Application Workflow
- Standard refs: GLI-13 significant system alterations and change-control expectations
- Code: `api/proto/rgs/v1/config.proto`, `internal/platform/server/config_grpc.go`, `migrations/000005_config_change_control.up.sql`
- Tests: `internal/platform/server/config_grpc_test.go`
- Status: implemented (strict production disables in-memory config/download mirrors and relies on DB-backed retrieval paths)

## RGS-0402 Immutable Configuration History and Current Value Tracking
- Standard refs: GLI-13 alteration history/reportability requirements
- Code: `internal/platform/server/config_grpc.go`, `migrations/000005_config_change_control.up.sql`
- Tests: `internal/platform/server/config_grpc_test.go`
- Status: implemented

## RGS-0403 Download Library Activity Logging (Add/Update/Delete/Activate)
- Standard refs: GLI-21/GLI-13 style download and activity logging expectations
- Code: `api/proto/rgs/v1/config.proto`, `internal/platform/server/config_grpc.go`, `migrations/000005_config_change_control.up.sql`
- Tests: `internal/platform/server/config_grpc_test.go`
- Status: implemented

## RGS-0404 gRPC and REST Parity (Config and Download-Control)
- Standard refs: internal platform requirement (protobuf canonical API mirrored over REST)
- Code: `api/proto/rgs/v1/config.proto`, `cmd/rgsd/main.go`
- Tests: `internal/platform/server/config_gateway_test.go`
- Status: implemented

## RGS-0501 Remote Access Boundary Enforcement for Admin Paths
- Standard refs: GLI-21 remote access restrictions and controlled external access
- Code: `internal/platform/server/remote_access.go`, `cmd/rgsd/main.go`, `docs/deployment/FIREWALL_LOGGING.md`
- Tests: `internal/platform/server/remote_access_test.go`, `internal/platform/server/chaos_test.go`
- Status: implemented

## RGS-0502 Remote Access Activity Logging Semantics
- Standard refs: GLI-21 firewall/connection attempt logging expectations
- Code: `internal/platform/server/remote_access.go`, `internal/platform/server/metrics.go`, `cmd/rgsd/main.go`, `docs/deployment/FIREWALL_LOGGING.md`, `docs/deployment/METRICS_ALERTING.md`
- Tests: `internal/platform/server/remote_access_test.go`
- Status: implemented (DB-backed persistence, decision/log-cap observability metrics including `logging_unavailable`, and strict-mode fail-closed guard when admin-path logging persistence is unavailable or in-memory activity log cap is exhausted)

## RGS-0503 Chaos Tests for Loss Handling and Fail-Closed Degradation
- Standard refs: GLI-13 communication loss and fail-safe behavior expectations
- Code: `internal/platform/server/events_grpc.go`, `internal/platform/server/ledger_grpc.go`, `internal/platform/server/remote_access.go`
- Tests: `internal/platform/server/chaos_test.go`
- Status: implemented

## RGS-0504 Explicit Audit API for Regulator/Operator Retrieval
- Standard refs: GLI-13 significant event/alteration reporting and audit retrieval expectations
- Code: `api/proto/rgs/v1/audit.proto`, `internal/platform/server/audit_grpc.go`, `cmd/rgsd/main.go`
- Tests: `internal/platform/server/audit_grpc_test.go`
- Status: implemented

## RGS-0601 mTLS/TLS Runtime Enforcement Controls
- Standard refs: GLI-13/GLI-21 secure communications and controlled remote access channels
- Code: `internal/platform/server/tls.go`, `cmd/rgsd/main.go`
- Tests: `internal/platform/server/remote_access_test.go`
- Status: implemented

## RGS-0602 PostgreSQL-backed Config/Download Persistence Path
- Standard refs: GLI-13 change-control retention and download activity logging expectations
- Code: `internal/platform/server/config_grpc.go`, `internal/platform/server/config_postgres.go`, `migrations/000005_config_change_control.up.sql`, `cmd/rgsd/main.go`
- Tests: `internal/platform/server/config_grpc_test.go`, `internal/platform/server/config_gateway_test.go`
- Status: implemented (optional runtime path via `RGS_DATABASE_URL`)

## RGS-0603 JWT Key Rotation with Active/Previous Key Validation
- Standard refs: GLI-21 secure remote access/authentication controls, GLI-13 secure communications controls
- Code: `internal/platform/auth/jwt.go`, `internal/platform/server/identity_grpc.go`, `cmd/rgsd/main.go`
- Tests: `internal/platform/auth/jwt_test.go`
- Status: implemented (`kid`-based keyset with active signing key and verification across keyset)

## RGS-0604 Context-Bound Actor Authorization from JWT
- Standard refs: GLI-21 remote access user control and unauthorized access prevention
- Code: `internal/platform/auth/grpc_jwt.go`, `internal/platform/auth/jwt.go`, `internal/platform/server/actor_context.go`, `internal/platform/server/ledger_grpc.go`, `internal/platform/server/config_grpc.go`, `internal/platform/server/events_grpc.go`, `internal/platform/server/registry_grpc.go`, `internal/platform/server/reporting_grpc.go`, `internal/platform/server/audit_grpc.go`
- Tests: `internal/platform/server/identity_grpc_test.go`, `internal/platform/server/ledger_grpc_test.go`, `internal/platform/server/config_grpc_test.go`
- Status: implemented (token actor preferred; request actor mismatch denied)

## RGS-0605 Identity Credential Management and Lockout Controls
- Standard refs: GLI-13 workstation/account controls and lockout expectations, GLI-21 unauthorized access prevention
- Code: `api/proto/rgs/v1/identity.proto`, `internal/platform/server/identity_grpc.go`, `internal/platform/server/identity_postgres.go`, `migrations/000006_identity_auth.up.sql`, `cmd/credhash/main.go`, `cmd/rgsd/main.go`
- Tests: `internal/platform/server/identity_grpc_test.go`, `internal/platform/server/postgres_integration_test.go`
- Status: implemented (credential set/rotation, enable/disable controls, bcrypt verification, lockout status/reset operations, failed-attempt lockout policy)

## RGS-0606 Refresh Session Revocation, Persistence, and Expiry Sweep
- Standard refs: GLI-13 session/account control expectations and retention controls
- Code: `internal/platform/server/identity_grpc.go`, `internal/platform/server/identity_postgres.go`, `migrations/000007_identity_sessions.up.sql`, `cmd/rgsd/main.go`
- Tests: `internal/platform/server/identity_grpc_test.go`, `internal/platform/server/postgres_integration_test.go`
- Status: implemented (DB-backed refresh sessions, revoke/rotate workflow, background expiry cleanup)

## RGS-0607 Bootstrap Credential Enforcement in DB-backed Mode
- Standard refs: GLI-21 restricted startup/admin access hardening expectations
- Code: `cmd/rgsd/main.go`, `internal/platform/server/identity_postgres.go`, `migrations/000006_identity_auth.up.sql`
- Tests: `internal/platform/server/postgres_integration_test.go`
- Status: implemented (startup fails when DB mode is enabled and no active credentials exist)

## RGS-0608 Identity Authentication Observability and Alerting
- Standard refs: GLI-13/GLI-21 operational monitoring and security event review expectations
- Code: `internal/platform/server/metrics.go`, `cmd/rgsd/main.go`, `docs/deployment/METRICS_ALERTING.md`
- Tests: `internal/platform/server/identity_grpc_test.go`, `internal/platform/server/postgres_integration_test.go`
- Status: implemented (login outcome counters, lockout activation counters, session-state gauges, and baseline alert rules)

## RGS-0609 No Plaintext Credential Material Over Identity Admin API
- Standard refs: GLI-21 secure communications/authentication handling, GLI-13 secure protocol-boundary controls
- Code: `api/proto/rgs/v1/identity.proto`, `internal/platform/server/identity_grpc.go`, `cmd/credhash/main.go`, `README.md`
- Tests: `internal/platform/server/identity_grpc_test.go`, `internal/platform/server/postgres_integration_test.go`
- Status: implemented (`SetCredential` accepts bcrypt `credential_hash` only, rejects non-bcrypt and low-cost hashes, and does not accept plaintext secrets)

## RGS-0701 Wagering Lifecycle API Surface and Idempotency
- Standard refs: GLI-13 event/state traceability objectives and AGENTS wagering scope requirements
- Code: `api/proto/rgs/v1/wagering.proto`, `internal/platform/server/wagering_grpc.go`, `internal/platform/server/wagering_postgres.go`, `migrations/000010_wagering_persistence.up.sql`, `cmd/rgsd/main.go`
- Tests: `internal/platform/server/wagering_grpc_test.go`, `internal/platform/server/postgres_integration_test.go`
- Status: implemented (place/settle/cancel with actor authorization, idempotency, audit records, and DB-backed replay/durability in PostgreSQL mode; strict production disables in-memory idempotency and wager-state mirrors in DB mode)

## RGS-0702 Strict Production Runtime Guardrails
- Standard refs: GLI-21 secure deployment and access-channel hardening expectations
- Code: `cmd/rgsd/main.go`, `cmd/rgsd/main_test.go`, `README.md`
- Tests: `cmd/rgsd/main_test.go`
- Status: implemented (`RGS_STRICT_PRODUCTION_MODE` requires DB + TLS + non-default JWT signing setup)

## RGS-0703 Account Transaction Statement Reporting
- Standard refs: GLI-16 account/transaction statement and regulator reporting expectations
- Code: `api/proto/rgs/v1/reporting.proto`, `internal/platform/server/reporting_grpc.go`, `internal/platform/server/reporting_postgres.go`, `docs/compliance/REPORT_CATALOG.md`
- Tests: `internal/platform/server/reporting_grpc_test.go`
- Status: implemented (DTD/MTD/YTD/LTD and JSON/CSV support)

## RGS-0704 Durable Remote Access Activity Retention
- Standard refs: GLI-21 connection-attempt logging and remote access review expectations
- Code: `internal/platform/server/remote_access.go`, `internal/platform/server/audit_grpc.go`, `migrations/000008_remote_access_activity.up.sql`, `cmd/rgsd/main.go`
- Tests: `internal/platform/server/remote_access_test.go`, `internal/platform/server/postgres_integration_test.go`
- Status: implemented (DB-backed remote access activity retrieval in PostgreSQL mode; strict production disables in-memory remote-access activity cache)

## RGS-0705 Bonusing/Promotions/UI Overlay Recall Scaffolds
- Standard refs: GLI-13/GLI-28/GLI-16 scaffold requirements from AGENTS for bonus/promotional meters and system-window recall
- Code: `api/proto/rgs/v1/extensions.proto`, `migrations/000009_bonus_ui_scaffolds.up.sql`, `docs/deployment/WIRELESS_ONBOARDING.md`, `README.md`
- Tests: `internal/platform/server/postgres_integration_test.go` (schema reset includes scaffold tables)
- Status: superseded by `RGS-0707` runtime implementation (contracts/scaffolds retained as underlying artifacts)

## RGS-0706 Production Evidence Packet Index
- Standard refs: GLI-13/GLI-21 evidence and auditability expectations
- Code: `docs/compliance/PRODUCTION_EVIDENCE_CHECKLIST.md`, `docs/compliance/PRODUCTION_READINESS_ROUNDS.md`
- Tests: `go test ./...` release-gate execution
- Status: implemented (release evidence checklist defined and linked)

## RGS-0707 Promotions and UI Overlay Runtime Services
- Standard refs: GLI-16 bonusing/promotional tracking expectations, GLI-28 system-window recall expectations
- Code: `api/proto/rgs/v1/extensions.proto`, `internal/platform/server/extensions_grpc.go`, `internal/platform/server/extensions_postgres.go`, `cmd/rgsd/main.go`, `migrations/000009_bonus_ui_scaffolds.up.sql`
- Tests: `internal/platform/server/extensions_grpc_test.go`, `internal/platform/server/extensions_gateway_test.go`
- Status: implemented (record/list bonus transactions, record/list promotional awards, submit/list system-window events with DB-backed persistence when configured; strict production disables in-memory promotions/UI mirrors)

## RGS-0708 JWT Keyset File Source and Live Reload
- Standard refs: GLI-21 secure remote access/authentication controls and key lifecycle hardening expectations
- Code: `internal/platform/auth/jwt.go`, `internal/platform/auth/keyset_source.go`, `cmd/rgsd/main.go`, `docs/deployment/KEY_MANAGEMENT.md`
- Tests: `internal/platform/auth/jwt_test.go`, `internal/platform/auth/keyset_source_test.go`, `cmd/rgsd/main_test.go`
- Status: implemented (file/command-backed keyset loading and in-process signer/verifier hot reload with periodic refresh; strict mode can require external keyset sources)

## RGS-0709 Identity Rate Limiting and EFT Fraud Lockout Controls
- Standard refs: GLI-21 unauthorized access prevention and GLI-16 EFT fraud lockout guidance
- Code: `internal/platform/server/identity_grpc.go`, `internal/platform/server/ledger_grpc.go`, `internal/platform/server/postgres_integration_test.go`, `migrations/000012_identity_login_rate_limits.up.sql`, `migrations/000013_ledger_eft_lockouts.up.sql`, `cmd/rgsd/main.go`, `README.md`
- Tests: `internal/platform/server/identity_grpc_test.go`, `internal/platform/server/postgres_integration_test.go`, `internal/platform/server/ledger_grpc_test.go`
- Status: implemented (configurable login rate limits with DB-backed rate-limit state in PostgreSQL mode and configurable EFT fraud lockouts with DB-backed lockout state in PostgreSQL mode)

## RGS-0710 Signed Download Activation Verification
- Standard refs: GLI-21/GLI-13 download activation and change integrity expectations
- Code: `api/proto/rgs/v1/config.proto`, `internal/platform/server/config_grpc.go`, `internal/platform/server/config_postgres.go`, `cmd/rgsd/main.go`, `migrations/000011_download_signature_verification.up.sql`, `docs/deployment/PACKAGE_SIGNING.md`
- Tests: `internal/platform/server/config_grpc_test.go`, `cmd/rgsd/main_test.go`
- Status: implemented (activation entries require valid signer id and package signature verification before acceptance)

## RGS-0711 Service-Level Request SLO Observability
- Standard refs: GLI-13/GLI-21 operational monitoring and security event review expectations
- Code: `internal/platform/server/metrics.go`, `cmd/rgsd/main.go`, `docs/deployment/METRICS_ALERTING.md`
- Tests: `internal/platform/server/metrics_observability_test.go`
- Status: implemented (gRPC and REST request outcome/latency metrics with baseline failure-rate and latency alert guidance)

## RGS-0712 Backup/Restore Drill Evidence Automation
- Standard refs: GLI-13 critical data retention/recovery and operational resilience expectations
- Code: `scripts/dr_backup_restore_check.sh`, `docs/deployment/DR_DRILLS.md`, `Makefile`, `docs/compliance/PRODUCTION_EVIDENCE_CHECKLIST.md`
- Tests: `go test ./...` release-gate execution
- Status: implemented (repeatable script-driven backup hash/count artifact pack with optional drill restore target)

## RGS-0713 Performance Qualification Baseline Evidence
- Standard refs: GLI-13 operational resilience and deterministic behavior verification objectives
- Code: `internal/platform/server/ledger_benchmark_test.go`, `scripts/perf_slo_check.sh`, `docs/deployment/PERFORMANCE_QUALIFICATION.md`, `Makefile`
- Tests: `go test ./internal/platform/server -run '^$' -bench '^BenchmarkLedgerDeposit$' -benchmem`
- Status: implemented (repeatable ledger benchmark artifacts with optional threshold gate for release evidence)

## RGS-0714 Failover/Partition RTO-RPO Evidence Snapshots
- Standard refs: GLI-13 communication loss/recovery and critical data resilience expectations
- Code: `scripts/failover_evidence_snapshot.sh`, `docs/deployment/FAILOVER_DRILLS.md`, `Makefile`, `docs/compliance/PRODUCTION_EVIDENCE_CHECKLIST.md`
- Tests: `go test ./...` release-gate execution
- Status: implemented (scripted failover snapshot capture with explicit RTO/RPO computation and threshold pass/fail gate)

## RGS-0715 Key Rotation Evidence Pack for External Key Custody
- Standard refs: GLI-21 secure key lifecycle controls and GLI-13 operational auditability expectations
- Code: `scripts/keyset_rotation_evidence.sh`, `docs/deployment/KEY_MANAGEMENT.md`, `Makefile`, `docs/compliance/PRODUCTION_EVIDENCE_CHECKLIST.md`
- Tests: `go test ./...` release-gate execution
- Status: implemented (scripted keyset snapshot/fingerprint evidence with active-kid rotation-state capture)

## RGS-0716 Sustained Load/Soak Qualification Evidence
- Standard refs: GLI-13 deterministic operation and operational resilience evidence expectations
- Code: `internal/platform/server/ledger_benchmark_test.go`, `internal/platform/server/wagering_benchmark_test.go`, `internal/platform/server/postgres_benchmark_test.go`, `scripts/load_soak_check.sh`, `scripts/load_soak_check_db.sh`, `scripts/load_soak_matrix.sh`, `docs/deployment/LOAD_SOAK_QUALIFICATION.md`, `Makefile`
- Tests: `go test ./internal/platform/server -run '^$' -bench '^(BenchmarkLedgerDeposit|BenchmarkWageringPlaceWager|BenchmarkLedgerDepositPostgres|BenchmarkWageringPlaceWagerPostgres)$' -benchmem`
- Status: implemented (multi-run soak benchmarks with optional threshold gating, DB-backed durability-path qualification, and operator-class profile matrix evidence artifacts)

## RGS-0717 Player Session Management API and Durability
- Standard refs: AGENTS core RGS capability requirement (player session management with timeout/device binding) and GLI-13 session/account control expectations
- Code: `api/proto/rgs/v1/sessions.proto`, `internal/platform/server/sessions_grpc.go`, `internal/platform/server/sessions_postgres.go`, `migrations/000014_player_sessions.up.sql`, `cmd/rgsd/main.go`, `README.md`
- Tests: `internal/platform/server/sessions_grpc_test.go`, `internal/platform/server/sessions_gateway_test.go`, `internal/platform/server/postgres_integration_test.go`
- Status: implemented (start/get/end session APIs, timeout-driven expiration transition on retrieval, actor-bound authorization, audit records, and DB-backed persistence/restart retrieval in PostgreSQL mode)

## RGS-0718 DB-backed Audit Event Persistence and Retrieval
- Standard refs: GLI-13 significant event/alteration retention and regulator retrieval expectations
- Code: `api/proto/rgs/v1/audit.proto`, `internal/platform/server/audit_postgres.go`, `internal/platform/server/audit_grpc.go`, `internal/platform/server/ledger_grpc.go`, `internal/platform/server/wagering_grpc.go`, `internal/platform/server/identity_grpc.go`, `internal/platform/server/registry_grpc.go`, `internal/platform/server/events_grpc.go`, `internal/platform/server/reporting_grpc.go`, `internal/platform/server/config_grpc.go`, `internal/platform/server/extensions_grpc.go`, `internal/platform/server/sessions_grpc.go`, `internal/platform/server/remote_access.go`, `cmd/rgsd/main.go`
- Tests: `internal/platform/server/audit_grpc_test.go`, `internal/platform/server/postgres_integration_test.go` (`TestPostgresAuditChainVerificationPassesForPersistedEvents`, `TestPostgresAuditChainVerificationDetectsTamper`, `TestPostgresAuditServiceVerifyAuditChain`)
- Status: implemented (DB-enabled core services and remote-access outcomes persist audit appends to `audit_events` with hash chaining semantics, `AuditService/ListAuditEvents` reads DB-backed records when configured, and `AuditService/VerifyAuditChain` provides an explicit regulator/operator verification API with input validation that detects tamper/mismatch conditions)

## RGS-0719 Audit Chain Verification Evidence Automation
- Standard refs: GLI-13 audit immutability verification and regulator evidence expectations
- Code: `scripts/audit_chain_evidence.sh`, `docs/deployment/AUDIT_CHAIN_VERIFICATION.md`, `Makefile`, `docs/compliance/PRODUCTION_EVIDENCE_CHECKLIST.md`, `docs/compliance/GO_LIVE_CHECKLIST.md`
- Tests: `go test ./...` release-gate execution
- Status: implemented (repeatable API-driven artifact capture for single or multi-partition audit-chain verification runs)
