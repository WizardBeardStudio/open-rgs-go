# open-rgs-go

GLI-aligned Remote Gaming System (RGS) backend in Go, exposing canonical protobuf APIs over gRPC and REST (grpc-gateway).

This manual covers build, deployment, operations, security controls, and runbooks for the current implementation state (Phases 0-5).

## 1. Implementation Status

Implemented and wired:
- `SystemService`
- `IdentityService` (player/operator login, refresh, logout)
- `LedgerService` (cashless semantics, idempotency, invariants)
- `WageringService` (wager placement, settlement, cancellation)
- `RegistryService` (equipment registry)
- `EventsService` (significant events/meters with buffering semantics)
- `ReportingService` (DTD/MTD/YTD/LTD, JSON/CSV)
- `ConfigService` (propose/approve/apply workflow + download-library logs)
- `AuditService` (audit event retrieval + remote-access activity retrieval)
- `PromotionsService` (bonus transactions + promotional award capture)
- `UISystemOverlayService` (system-window open/close recall event ingestion and listing)

Current persistence model:
- Runtime services support optional PostgreSQL-backed paths when `RGS_DATABASE_URL` is configured.
- DB-backed paths currently include ledger reads/writes and idempotency replay, wagering state and idempotency replay, registry reads/writes, events/meters reads/writes, reporting run persistence and report payload sourcing, config/download change-control reads/writes, and remote access activity retention.
- Identity credential verification and lockout state use PostgreSQL tables when configured (`identity_credentials`, `identity_lockouts`).
- In-memory behavior remains available as a fallback for local/dev execution without PostgreSQL.

## 2. Repository Layout

- `api/proto/rgs/v1/`: canonical API contracts
- `gen/rgs/v1/`: generated Go/gRPC/gateway stubs
- `cmd/rgsd/`: server entrypoint
- `internal/platform/server/`: service implementations
- `internal/platform/audit/`: audit model + hash chaining
- `migrations/`: SQL schema evolution
- `docs/compliance/`: traceability, report catalog, threat model
- `docs/deployment/`: deployment hardening guidance

## 3. Prerequisites

Required:
- Go (version from `go.mod`)
- Buf CLI (`buf`)

Optional but recommended:
- PostgreSQL (for applying migrations and validating DB schema)
- `golangci-lint`

## 4. Build and Test

From `src/`:

```bash
go test ./...
```

PostgreSQL integration tests (restart/durability scenarios):

```bash
RGS_TEST_DATABASE_URL="postgres://user:pass@localhost:5432/open_rgs_go_test?sslmode=disable" \
go test ./internal/platform/server -run '^TestPostgres'
# or
make test-integration-postgres
```

Credential hash tool:

```bash
go run ./cmd/credhash "plain-secret"
# or
printf 'plain-secret\n' | go run ./cmd/credhash
```

Format + tests:

```bash
make all
# or
./scripts/check.sh
```

Lint (if installed):

```bash
make lint
```

## 5. Protobuf and Gateway Generation

Contracts are source-of-truth in `api/proto/rgs/v1/`.

```bash
buf lint
buf generate
```

Generated artifacts are committed under `gen/rgs/v1/`.

## 6. Database Migrations

Schema files are ordered and additive:
- `000001_init_core.*` audit core
- `000002_ledger_cashless.*` ledger/cashless
- `000003_registry_events_meters.*` registry/events/meters/buffering
- `000004_reporting_runs.*` report persistence
- `000005_config_change_control.*` config/download change-control
- `000006_identity_auth.*` identity credentials and lockout tracking
- `000007_identity_sessions.*` refresh-session persistence and cleanup
- `000008_remote_access_activity.*` remote access activity persistence
- `000009_bonus_ui_scaffolds.*` bonusing/promotions/UI overlay recall scaffolds
- `000010_wagering_persistence.*` wagering durability and idempotency persistence
- `000011_download_signature_verification.*` signed download activation verification fields/indexes

Apply migrations with your preferred migration runner in numeric order.

## 7. Runtime Configuration

Environment variables:
- `RGS_VERSION` (default: `dev`)
- `RGS_GRPC_ADDR` (default: `:8081`)
- `RGS_HTTP_ADDR` (default: `:8080`)
- `RGS_TRUSTED_CIDRS` (default: `127.0.0.1/32,::1/128`)
- `RGS_DATABASE_URL` (optional PostgreSQL DSN for config/download persistence)
- `RGS_STRICT_PRODUCTION_MODE` (default: `true` when `RGS_VERSION != dev`, otherwise `false`; when enabled, startup requires DB + TLS + non-default JWT signing setup)
- `RGS_STRICT_EXTERNAL_JWT_KEYSET` (default: same as `RGS_STRICT_PRODUCTION_MODE`; when enabled, startup requires `RGS_JWT_KEYSET_FILE` or `RGS_JWT_KEYSET_COMMAND`)
- `RGS_JWT_SIGNING_SECRET` (default: `dev-insecure-change-me`; HMAC key for identity access tokens)
- `RGS_JWT_KEYSET` (optional; comma-separated `kid:secret` entries for key rotation, e.g. `old:secret1,new:secret2`)
- `RGS_JWT_ACTIVE_KID` (default: `default`; active signing key id from `RGS_JWT_KEYSET`)
- `RGS_JWT_KEYSET_FILE` (optional; JSON keyset file path, intended for KMS/HSM sidecar-managed key material)
- `RGS_JWT_KEYSET_COMMAND` (optional; command that returns keyset JSON payload, for KMS/HSM client integration)
- `RGS_JWT_KEYSET_REFRESH_INTERVAL` (default: `1m`; when `RGS_JWT_KEYSET_FILE` or `RGS_JWT_KEYSET_COMMAND` is set, reload cadence for live signer/verifier rotation)
- `RGS_DOWNLOAD_SIGNING_KEYS` (optional; comma-separated `kid:secret` keys used to verify download-library activation signatures)
- `RGS_JWT_ACCESS_TTL` (default: `15m`)
- `RGS_JWT_REFRESH_TTL` (default: `24h`)
- `RGS_IDENTITY_LOCKOUT_MAX_FAILURES` (default: `5`)
- `RGS_IDENTITY_LOCKOUT_TTL` (default: `15m`)
- `RGS_IDENTITY_LOGIN_RATE_LIMIT_MAX_ATTEMPTS` (default: `60`; per-actor login attempts allowed per rate-limit window)
- `RGS_IDENTITY_LOGIN_RATE_LIMIT_WINDOW` (default: `1m`; rolling window for login rate limiting)
- `RGS_IDENTITY_SESSION_CLEANUP_INTERVAL` (default: `15m`)
- `RGS_IDENTITY_SESSION_CLEANUP_BATCH` (default: `500`)
- `RGS_EFT_FRAUD_MAX_FAILURES` (default: `5`; repeated denied EFT operations before lockout)
- `RGS_EFT_FRAUD_LOCKOUT_TTL` (default: `15m`; lockout duration after fraud threshold reached)
- `RGS_TEST_DATABASE_URL` (optional PostgreSQL DSN for env-gated integration tests)
- `RGS_LEDGER_IDEMPOTENCY_TTL` (default: `24h`; retention window for idempotency envelopes)
- `RGS_LEDGER_IDEMPOTENCY_CLEANUP_INTERVAL` (default: `15m`; cleanup worker cadence)
- `RGS_LEDGER_IDEMPOTENCY_CLEANUP_BATCH` (default: `500`; max expired keys deleted per cleanup batch)
- `RGS_METRICS_REFRESH_INTERVAL` (default: `1m`; refresh cadence for DB-backed metrics gauges)
- `RGS_TLS_ENABLED` (`true|false`, default: `false`)
- `RGS_TLS_CERT_FILE` (required when TLS enabled)
- `RGS_TLS_KEY_FILE` (required when TLS enabled)
- `RGS_TLS_REQUIRE_CLIENT_CERT` (`true|false`, default: `false`)
- `RGS_TLS_CLIENT_CA_FILE` (required when client certs are required)

Example:

```bash
RGS_VERSION=1.0.0 \
RGS_GRPC_ADDR=:8081 \
RGS_HTTP_ADDR=:8080 \
RGS_TRUSTED_CIDRS="10.0.0.0/8,192.168.0.0/16,127.0.0.1/32,::1/128" \
RGS_DATABASE_URL="postgres://user:pass@localhost:5432/open_rgs_go?sslmode=disable" \
RGS_TLS_ENABLED=true \
RGS_TLS_CERT_FILE=./certs/server.crt \
RGS_TLS_KEY_FILE=./certs/server.key \
RGS_TLS_REQUIRE_CLIENT_CERT=true \
RGS_TLS_CLIENT_CA_FILE=./certs/clients_ca.pem \
go run ./cmd/rgsd
```

## 8. Start and Verify

Start server:

```bash
go run ./cmd/rgsd
```

Health check:

```bash
curl -i http://127.0.0.1:8080/healthz
```

Prometheus metrics:

```bash
curl -s http://127.0.0.1:8080/metrics
```

System status (REST via gateway):

```bash
curl -s http://127.0.0.1:8080/v1/system/status | jq
```

## 9. Security and Remote Access Controls

Remote admin boundary:
- Admin-style paths are guarded by trusted CIDRs:
  - `/v1/config/*`
  - `/v1/reporting/*`
  - `/v1/audit/*` (when exposed)
- Untrusted sources receive `403`.

Additional controls:
- Actor-bound authZ checks in services (`player`, `operator`, `service`)
- Protected HTTP/gRPC calls derive actor identity from JWT middleware/interceptor context; request `meta.actor` mismatch with token is denied.
- Append-only audit chain semantics
- Fail-closed behavior on critical audit unavailability for state-changing operations
- Ingestion buffer exhaustion disables further ingress for affected boundary

Deployment guidance:
- `docs/deployment/FIREWALL_LOGGING.md`
- `docs/deployment/METRICS_ALERTING.md`
- `docs/deployment/WIRELESS_ONBOARDING.md`
- `docs/deployment/KEY_MANAGEMENT.md`
- `docs/deployment/PACKAGE_SIGNING.md`
- `docs/deployment/DR_DRILLS.md`
- `docs/deployment/PERFORMANCE_QUALIFICATION.md`
- `docs/deployment/FAILOVER_DRILLS.md`
- `docs/deployment/LOAD_SOAK_QUALIFICATION.md`

## 10. API Surface (Current)

Services and methods are defined in:
- `api/proto/rgs/v1/system.proto`
- `api/proto/rgs/v1/identity.proto`
- `api/proto/rgs/v1/ledger.proto`
- `api/proto/rgs/v1/wagering.proto`
- `api/proto/rgs/v1/registry.proto`
- `api/proto/rgs/v1/events.proto`
- `api/proto/rgs/v1/reporting.proto`
- `api/proto/rgs/v1/config.proto`
- `api/proto/rgs/v1/audit.proto`
- `api/proto/rgs/v1/extensions.proto`

Cross-cutting request/response metadata is in `api/proto/rgs/v1/common.proto`.

Identity admin flow:
- `IdentityService/SetCredential` is restricted to operator/service actors and requires DB persistence.
- Use it to create/rotate player and operator credentials with bcrypt hashes only (`credential_hash`); plaintext credential material is never accepted by the API.
- When `RGS_DATABASE_URL` is configured, startup fails if no active rows exist in `identity_credentials`.

## 11. Operations Runbook

### Deployment Checklist
- Apply latest migrations.
- Set `RGS_TRUSTED_CIDRS` for your trusted ops network.
- Verify `/healthz` and `/v1/system/status`.
- Run smoke checks for at least one endpoint per major service.

### Identity Credential Seeding
- Apply `000006_identity_auth.*` migrations.
- Generate a bcrypt hash:
  - `go run ./cmd/credhash "initial-password"`
- Insert bootstrap credentials directly (one-time) or call `POST /v1/identity/credentials:set` as an authenticated operator/service with a precomputed bcrypt `credential_hash`.
- Example payload (hash truncated):
  - `{"meta":{"request_id":"...","actor":{"actor_id":"op-1","actor_type":"ACTOR_TYPE_OPERATOR"}},"actor":{"actor_id":"player-1","actor_type":"ACTOR_TYPE_PLAYER"},"credential_hash":"$2a$10$...","reason":"bootstrap"}`
- In DB-backed mode, legacy fallback credentials are not used.

### Post-Deploy Validation
- `go test ./...` in CI is green.
- Gateway parity tests are green.
- Remote admin path denied from untrusted source and allowed from trusted source.

### Incident/Fault Scenarios
- Lost comms/buffer exhaustion: events ingress should deny and disable boundary.
- Audit-store unavailability: critical state changes should fail closed.
- Untrusted remote admin attempts: denied and logged.

Chaos tests:
- `internal/platform/server/chaos_test.go`

## 12. Compliance Artifacts

- Requirements traceability: `docs/compliance/REQUIREMENTS.md`
- Threat model: `docs/compliance/THREAT_MODEL.md`
- Report catalog: `docs/compliance/REPORT_CATALOG.md`
- Production readiness rounds: `docs/compliance/PRODUCTION_READINESS_ROUNDS.md`
- Production evidence checklist: `docs/compliance/PRODUCTION_EVIDENCE_CHECKLIST.md`
- Go-live release gates: `docs/compliance/GO_LIVE_CHECKLIST.md`

## 13. Known Limitations and Next Work

Current limitations:
- Some non-authoritative operational mirrors remain in-memory for performance, with PostgreSQL as system-of-record where configured; strict production mode disables in-memory idempotency replay caches for ledger/wagering, disables in-memory remote-access activity caching, and disables in-memory reporting fallback/run retention.
- JWT issuance/refresh/rotation is implemented, including live keyset-file reload hooks, but full KMS/HSM operational integration and key custody controls remain deployment responsibilities.
- Promotions/UI services are implemented at baseline CRUD/reportability level, but advanced campaign policy engines and full device-side interaction workflows are still pending.

Recommended next steps:
- Extend soak coverage to DB-backed distributed scenarios with target-concurrency profiles per jurisdiction/operator class.
