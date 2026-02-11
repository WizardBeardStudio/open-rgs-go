# Compliance Requirements Traceability (Scaffold)

This file is intentionally scaffolded in Phase 0 and will be expanded in feature phases.

## RGS-0001 Protocol Boundary and Authn/Authz
- Standard refs: GLI-13 / GLI-21 (remote access and controlled interfaces)
- Code: `cmd/rgsd/main.go`, `internal/platform/auth/jwt.go`
- Tests: `internal/platform/auth/jwt_test.go`
- Status: scaffolded

## RGS-0002 Append-only Audit Logging
- Standard refs: GLI-13 significant events and alterations
- Code: `internal/platform/audit/model.go`, `internal/platform/audit/chain.go`, `migrations/000001_init_core.up.sql`
- Tests: `internal/platform/audit/chain_test.go`
- Status: scaffolded

## RGS-0003 Deterministic Time Handling
- Standard refs: GLI-13 date/time reporting
- Code: `internal/platform/clock/clock.go`
- Tests: pending
- Status: scaffolded
