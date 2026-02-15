# Changelog

All notable changes to this package will be documented in this file.

## [0.1.0] - 2026-02-15

### Added
- Initial UPM package scaffold (`Runtime`, `Editor`, `Samples~`, `Documentation~`).
- Runtime bootstrap/config and facade (`RgsClientBootstrap`, `RgsClient`).
- gRPC-Web wiring for Identity, Ledger, Sessions, and Wagering core flows.
- REST gateway wiring for the same core flows.
- WebGL-safe REST runtime path via `UnityWebRequest`.
- Sample scripts:
  - `AuthAndBalanceSample`
  - `QuickStartSlotSample`
- Test scaffolding:
  - Editor tests (REST meta parsing, idempotency enforcement)
  - Runtime smoke tests (bootstrap error path, mock login/balance path)
