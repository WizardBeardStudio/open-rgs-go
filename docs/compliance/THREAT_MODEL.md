# Threat Model

## In Scope
- External API boundary for gRPC and REST
- Operator/service/player identity binding on every request
- Optional mTLS between clients and RGS
- Audit log integrity chain and append-only constraints
- Financial operations and configuration/change-control workflows
- Download library change and activation activity logging
- Remote admin boundary enforcement using trusted CIDR controls

## Key Threats
- Unauthorized API access via forged/replayed credentials
- Privilege escalation across actor types (player -> operator actions)
- Tampering with audit, configuration history, or download-library records
- Silent configuration mutation outside approved workflow
- Replay or duplicate execution of financial/config state changes
- Loss of event/meter communication causing data loss
- Unauthorized remote administrative access from untrusted networks

## Active Mitigations in Code
- JWT verification middleware with strict claim requirements
  - `internal/platform/auth/jwt.go`
- Actor-type authorization in stateful services
  - `internal/platform/server/ledger_grpc.go`
  - `internal/platform/server/wagering_grpc.go`
  - `internal/platform/server/registry_grpc.go`
  - `internal/platform/server/events_grpc.go`
  - `internal/platform/server/reporting_grpc.go`
  - `internal/platform/server/config_grpc.go`
- Append-only audit chain and immutability controls
  - `internal/platform/audit/*.go`
  - `migrations/000001_init_core.up.sql`
- Idempotency enforcement on financial operations
  - `internal/platform/server/ledger_grpc.go`
- Buffer + fail-closed disable behavior for ingestion exhaustion
  - `internal/platform/server/events_grpc.go`
- Configuration proposal/approval/apply workflow with immutable history
  - `internal/platform/server/config_grpc.go`
  - `migrations/000005_config_change_control.up.sql`
- Download-library change recording and recall log
  - `internal/platform/server/config_grpc.go`
  - `migrations/000005_config_change_control.up.sql`
- Remote access guard + admin-path filtering and activity logging
  - `internal/platform/server/remote_access.go`
  - `cmd/rgsd/main.go`
  - `docs/deployment/FIREWALL_LOGGING.md`
- DB-backed remote access activity retention path
  - `internal/platform/server/remote_access.go`
  - `migrations/000008_remote_access_activity.up.sql`
- Strict production startup guardrails (DB+TLS+non-default signing configuration)
  - `cmd/rgsd/main.go`
  - `cmd/rgsd/main_test.go`
- JWT keyset file loading and live in-process key reload
  - `internal/platform/auth/keyset_source.go`
  - `internal/platform/auth/jwt.go`
  - `cmd/rgsd/main.go`
  - `docs/deployment/KEY_MANAGEMENT.md`
- Explicit audit retrieval API (audit events + remote access activity)
  - `api/proto/rgs/v1/audit.proto`
  - `internal/platform/server/audit_grpc.go`
- TLS/mTLS runtime enforcement controls
  - `internal/platform/server/tls.go`
  - `cmd/rgsd/main.go`
- Optional PostgreSQL-backed persistence path for config/download controls
  - `internal/platform/server/config_postgres.go`
  - `cmd/rgsd/main.go`
- Scaffold contracts for promotions/bonusing/UI system-window recall
  - `api/proto/rgs/v1/extensions.proto`
  - `migrations/000009_bonus_ui_scaffolds.up.sql`
  - `docs/deployment/WIRELESS_ONBOARDING.md`

## Residual Risks / Follow-up
- Integrate persistent DB-backed service repositories (replace in-memory stores)
- Add full KMS/HSM custody integration and attested key lifecycle controls beyond file-based runtime hooks
- Add explicit rate-limiting, session lockout, and antifraud controls
- Add signed package verification pipeline for download library entries
- Expand promotions/bonusing/UI services beyond baseline record/list into full policy/workflow engines
