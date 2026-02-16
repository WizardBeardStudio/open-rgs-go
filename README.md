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
- `SessionsService` (player sessions, timeout state transitions, device binding)
- `PromotionsService` (bonus transactions + promotional award capture/listing)
- `UISystemOverlayService` (system-window open/close recall event ingestion and listing)

Current persistence model:
- Runtime services support optional PostgreSQL-backed paths when `RGS_DATABASE_URL` is configured.
- DB-backed paths currently include ledger reads/writes and idempotency replay, wagering state and idempotency replay, registry reads/writes, events/meters reads/writes, reporting run persistence and report payload sourcing, config/download change-control reads/writes, remote access activity retention, player session persistence, and audit event retrieval/writes for DB-enabled core state-changing operations.
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
- `docs/integration/`: client integration guides
- `examples/`: runnable integration examples

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
make check-module-path
go test ./...
# or
make test
# or
make verify
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

`make verify` runs module-path checks, proto freshness validation, and tests.
`./scripts/check.sh` runs formatting and then `make verify`.
The module-path check also validates `api/proto/rgs/v1/*.proto` `go_package` consistency.

Verification evidence pack:

```bash
make verify-evidence
```

This writes timestamped logs and a summary under `artifacts/verify/`.
When `GITHUB_ACTIONS=true`, `verify_evidence.sh` requires `RGS_VERIFY_EVIDENCE_PROTO_MODE=full` and `RGS_VERIFY_EVIDENCE_REQUIRE_CLEAN=true`.
Set `RGS_VERIFY_EVIDENCE_REQUIRE_CLEAN=true` to require a clean git worktree before evidence capture.
For CI-equivalent local execution, run:

```bash
make verify-evidence-strict
```

Strict/CI evidence runs require attestation signing key configuration:
- `RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID` (for example `ci-active`)
- `RGS_VERIFY_EVIDENCE_ATTESTATION_ALG=ed25519`
- `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY` (or ring: `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEYS`)
- `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEY` (or ring: `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEYS`)
- `RGS_VERIFY_EVIDENCE_ENFORCE_ATTESTATION_KEY=true` (set automatically by `make verify-evidence-strict`)

Example local strict run with a single ed25519 keypair:

```bash
RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID=local-active \
RGS_VERIFY_EVIDENCE_ATTESTATION_ALG=ed25519 \
RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY='<base64_private_or_seed>' \
RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEY='<base64_public>' \
make verify-evidence-strict
```

Example local strict run during key rotation:

```bash
RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID=active \
RGS_VERIFY_EVIDENCE_ATTESTATION_ALG=ed25519 \
RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEYS='active:<base64_priv_or_seed>,previous:<base64_priv_or_seed>' \
RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEYS='active:<base64_public>,previous:<base64_public>' \
make verify-evidence-strict
```

Attestation mode:
- Evidence attestation uses `ed25519`.

If Buf remote dependencies are unavailable in a local environment, use:

```bash
RGS_PROTO_CHECK_MODE=diff-only make proto-check
```

The same mode can be used for full local verification:

```bash
RGS_PROTO_CHECK_MODE=diff-only make verify
```

CI should continue using the default strict mode (`RGS_PROTO_CHECK_MODE=full`).
The repository CI workflow pins this mode explicitly for the proto job.
If `GITHUB_ACTIONS=true`, `scripts/check_proto_clean.sh` rejects any mode other than `full`.

### GitHub Actions Setup (for new clones/forks)

1. Push your clone/fork to GitHub and keep `.github/workflows/ci.yml` enabled.
2. In GitHub repo settings, allow workflow read access to repository contents (default for this workflow).
3. Add repository or organization secrets used by the `verify_evidence` job:
- `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY`
- `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEYS`
  Accepted secret formats:
  - single key: `<base64_key_material>`
  - key ring: `key_id:<base64_key_material>[,key_id2:<base64_key_material>]`
  Helper to generate compatible key-ring values:
  - `go run ./cmd/attestkeygen --key-id ci-active`
  - GitHub Secrets copy mode (prints only private/public values): `go run ./cmd/attestkeygen --key-id ci-active --format github-secrets`
  - Trusted-host file mode (writes restricted key files, prints `*_FILE` env assignments): `go run ./cmd/attestkeygen --key-id ci-active --out-dir ./tmp/attest-keys`
  - compatibility wrapper: `./scripts/gen_ci_attestation_keyring.sh ci-active`
  Useful environment defaults for the generator:
  - `RGS_ATTEST_KEYGEN_KEY_ID` (default: `ci-active`)
  - `RGS_ATTEST_KEYGEN_FORMAT` (`assignments` or `github-secrets`, default: `assignments`)
  - `RGS_ATTEST_KEYGEN_OUT_DIR` (optional output directory for file mode)
  - `RGS_ATTEST_KEYGEN_RING` (`true`/`false`, default: `true`)
  - `RGS_ATTEST_KEYGEN_PRIVATE_MATERIAL` (`seed` or `private`, default: `seed`)
4. Keep `RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID=ci-active` in workflow env, or set a different key id consistently with your public-key ring entry.
5. Open a PR and confirm these jobs pass:
- `test`
- `proto`
- `verify_evidence`

Notes:
- Do not commit attestation keys into the repo.
- The current workflow writes secret values to runner temp files and passes file paths to strict evidence verification.
- For rotation patterns and key-ring examples, see `docs/deployment/KEY_MANAGEMENT.md`.

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
- `000012_identity_login_rate_limits.*` DB-backed login rate limiting state
- `000013_ledger_eft_lockouts.*` DB-backed EFT fraud lockout state
- `000014_player_sessions.*` player session lifecycle persistence

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
- `RGS_REMOTE_ACCESS_ACTIVITY_LOG_CAP` (default: `5000`; max in-memory remote-access activity records before log-cap errors when DB logging is unavailable)
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
- Core and extension services audit denied/invalid requests with explicit denial reasons (including actor-binding failures such as `actor mismatch with token`), and parity tests assert this behavior across gRPC and REST gateway paths.
- Identity session/admin surfaces (`RefreshToken`, `Logout`, credential/lockout admin APIs) include explicit actor-ownership/binding denial checks with denied-audit assertions in gRPC and gateway tests.
- Identity admin authorization denials now emit explicit denied audit events (`identity_set_credential`, `identity_disable_credential`, `identity_enable_credential`, `identity_get_lockout`, `identity_reset_lockout`) for traceable operator/regulator review.
- Fail-closed behavior on critical audit unavailability for state-changing operations
- Strict production mode fail-closes admin-path access when remote-access logging persistence is unavailable
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
- `docs/deployment/AUDIT_CHAIN_VERIFICATION.md`

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
- `api/proto/rgs/v1/sessions.proto`
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
- In DB-backed mode, fallback credentials are not used.

### Post-Deploy Validation
- CI `test` and `proto` jobs are green (`make test` and strict `make proto-check`).
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
- GLI gap analysis: `docs/compliance/GLI_GAP_ANALYSIS.md`
- Unity/C# protobuf client guide: `docs/integration/UNITY_CSHARP_PROTO_CLIENT.md`
- Unity/C# runnable sample: `examples/unity-csharp-proto-client/README.md`
- Unity CI workflow (license-gated): `.github/workflows/unity-ci.yml`
- Unity package release workflow: `.github/workflows/unity-package-release.yml`
- `rgsd` binary release workflow (linux/amd64 + darwin/arm64): `.github/workflows/rgsd-release.yml`

## 13. Known Limitations and Next Work

Current limitations (must be dispositioned before production approval):
- In-memory operational mirrors are still present for non-production/runtime fallback paths.
- External key custody (KMS/HSM/Vault-backed process controls) is deployment-integrated, not product-enforced end-to-end.
- Promotions/UI support is baseline (CRUD/reportability), without advanced campaign policy orchestration.

Objective release-exit criteria for these limitations:
1. In-memory mirrors:
- `RGS_STRICT_PRODUCTION_MODE=true` in production.
- Startup evidence shows PostgreSQL configured and strict mode active.
- Evidence package includes pass outputs for `make verify`, `make verify-evidence-strict`, and DB qualification (`make soak-qual-db`, `make soak-qual-matrix`).
2. External key custody:
- Production deploy uses `RGS_JWT_KEYSET_FILE` or `RGS_JWT_KEYSET_COMMAND`.
- At least one current-cycle `make keyset-evidence` artifact is attached with operator/security sign-off.
- No inline JWT/attestation private key material in committed config or workflow YAML literals.
3. Promotions/UI scope:
- Either:
  - advanced campaign/device workflow requirements are implemented and tested, or
  - explicit deferred-scope sign-off is recorded in go-live evidence with jurisdiction/product approval.

Recommended next steps:
- Execute `make soak-qual-db` and `make soak-qual-matrix` in each DB-backed deployment tier and attach per-profile thresholds/baselines as release evidence.
- Execute `make keyset-evidence` for the active release cycle and attach sign-off notes.
- Complete Gate 8 and Gate 9 in `docs/compliance/GO_LIVE_CHECKLIST.md` with `PASS` status and linked artifacts.
- Optional helper for Gate 10 artifact collection:
  - `make gate10-evidence`
  - Strict mode (exit non-zero unless all Gate 10 inputs pass): `RGS_GATE10_FAIL_ON_INCOMPLETE=true make gate10-evidence`
  - Partial/local dry-run mode: `RGS_GATE10_SKIP_SOAK_DB=true RGS_GATE10_SKIP_SOAK_MATRIX=true RGS_GATE10_SKIP_KEYSET=true make gate10-evidence`
