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
- Status: implemented (in-memory service + schema)

## RGS-0202 Significant Event Ingestion with Time Triplet
- Standard refs: GLI-13 significant events and event retention/reporting
- Code: `api/proto/rgs/v1/events.proto`, `internal/platform/server/events_grpc.go`, `migrations/000003_registry_events_meters.up.sql`
- Tests: `internal/platform/server/registry_events_test.go`, `internal/platform/server/events_replay_test.go`
- Status: implemented (in-memory service + schema)

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
- Status: implemented

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
