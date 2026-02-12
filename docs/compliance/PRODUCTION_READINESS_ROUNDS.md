# Production Readiness Rounds

This checklist tracks the remaining production-readiness work identified against `AGENTS.md` scope and current implementation state.

## Round 1: Execution Checklist and Acceptance Gates
- Status: implemented
- Deliverable: this document with explicit acceptance criteria and evidence anchors.

## Round 2: Wagering Domain APIs and Service
- Goal: implement canonical wagering API surface and runtime behavior.
- Acceptance:
  - Add `WageringService` proto with `PlaceWager`, `SettleWager`, `CancelWager`.
  - Enforce actor authorization and idempotency for state-changing operations.
  - Register gRPC and REST gateway handlers.
  - Add unit tests for idempotency and lifecycle transitions.

## Round 3: Production Durability Enforcement
- Goal: prevent accidental non-durable production deployments.
- Acceptance:
  - Add startup policy checks for strict production mode.
  - Require PostgreSQL in strict production mode.
  - Require TLS in strict production mode.
  - Document strict-production environment variables and behavior.

## Round 4: Reporting Scope Expansion
- Goal: increase regulator-facing reporting coverage.
- Acceptance:
  - Add account transaction statement report type.
  - Support DTD/MTD/YTD/LTD and JSON/CSV output.
  - Add report test coverage for data and no-activity behavior.
  - Update report catalog documentation.

## Round 5: Durable Remote Access Activity Retention
- Goal: persist remote access activity logs in DB-backed deployments.
- Acceptance:
  - Add migration for remote access activity table.
  - Persist allow/deny activity records when DB is configured.
  - Ensure `ListRemoteAccessActivities` works across process restarts in DB mode.
  - Add env-gated integration test coverage.

## Round 6: Required Scaffolds (Bonusing/Promotions/Wireless/UI Overlay Recall)
- Goal: establish canonical contracts and storage hooks for AGENTS-required scaffolds.
- Acceptance:
  - Add protobuf scaffolds for promotions/bonusing and UI system window event recall.
  - Add DB schema scaffold for system-window recall events.
  - Add server-side wireless onboarding constraints documentation reference and compliance mapping entry.

## Round 7: Production Evidence Pack and Consistency Sweep
- Goal: provide regulator/operator-ready evidence index and eliminate documentation drift.
- Acceptance:
  - Add production evidence checklist (security, DR, chaos, perf, traceability).
  - Align README known-limitations with implemented identity/JWT behavior.
  - Update compliance requirements with new rounds and code/test pointers.
