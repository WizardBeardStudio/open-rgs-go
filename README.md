# open-rgs-go

GLI-aligned Remote Gaming System (RGS) backend in Go, exposing canonical protobuf APIs over gRPC and REST (grpc-gateway).

This manual covers build, deployment, operations, security controls, and runbooks for the current implementation state (Phases 0-5).

## 1. Implementation Status

Implemented and wired:
- `SystemService`
- `LedgerService` (cashless semantics, idempotency, invariants)
- `RegistryService` (equipment registry)
- `EventsService` (significant events/meters with buffering semantics)
- `ReportingService` (DTD/MTD/YTD/LTD, JSON/CSV)
- `ConfigService` (propose/approve/apply workflow + download-library logs)

Current persistence model:
- Runtime services currently use in-memory repositories for behavior.
- PostgreSQL migrations are provided for all core domains and are ready for DB-backed repositories.

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

Apply migrations with your preferred migration runner in numeric order.

## 7. Runtime Configuration

Environment variables:
- `RGS_VERSION` (default: `dev`)
- `RGS_GRPC_ADDR` (default: `:8081`)
- `RGS_HTTP_ADDR` (default: `:8080`)
- `RGS_TRUSTED_CIDRS` (default: `127.0.0.1/32,::1/128`)

Example:

```bash
RGS_VERSION=1.0.0 \
RGS_GRPC_ADDR=:8081 \
RGS_HTTP_ADDR=:8080 \
RGS_TRUSTED_CIDRS="10.0.0.0/8,192.168.0.0/16,127.0.0.1/32,::1/128" \
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
- Append-only audit chain semantics
- Fail-closed behavior on critical audit unavailability for state-changing operations
- Ingestion buffer exhaustion disables further ingress for affected boundary

Deployment guidance:
- `docs/deployment/FIREWALL_LOGGING.md`

## 10. API Surface (Current)

Services and methods are defined in:
- `api/proto/rgs/v1/system.proto`
- `api/proto/rgs/v1/ledger.proto`
- `api/proto/rgs/v1/registry.proto`
- `api/proto/rgs/v1/events.proto`
- `api/proto/rgs/v1/reporting.proto`
- `api/proto/rgs/v1/config.proto`

Cross-cutting request/response metadata is in `api/proto/rgs/v1/common.proto`.

## 11. Operations Runbook

### Deployment Checklist
- Apply latest migrations.
- Set `RGS_TRUSTED_CIDRS` for your trusted ops network.
- Verify `/healthz` and `/v1/system/status`.
- Run smoke checks for at least one endpoint per major service.

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

## 13. Known Limitations and Next Work

Current limitations:
- Service repositories are in-memory; DB schemas are ready but not yet fully wired.
- Remote access activity logs are in-memory unless connected to persistent sinks.

Recommended next steps:
- Replace in-memory stores with PostgreSQL repositories for each domain.
- Add production authN (JWT issuance/rotation + mTLS policy enforcement).
- Add explicit admin/audit API for remote-access activity retrieval.
