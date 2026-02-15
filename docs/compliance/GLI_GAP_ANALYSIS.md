# GLI Implementation Gap Analysis (Current State)

This analysis summarizes current implementation coverage against the GLI-oriented scope in `AGENTS.md`, using:
- `docs/compliance/REQUIREMENTS.md`
- `docs/compliance/THREAT_MODEL.md`
- `docs/compliance/GO_LIVE_CHECKLIST.md`
- current runtime/deployment docs under `docs/deployment/`

Note: This is an engineering gap analysis, not a certification result. Final conformity is determined by regulator/lab review against the cited GLI standards and test procedures.

## Coverage Snapshot by Standard Family

## GLI-13 (Monitoring/Control, Significant Events, Alterations, Reporting)
- Coverage status: strong implementation coverage.
- Implemented:
  - significant event/meter ingestion and storage paths
  - report generation with DTD/MTD/YTD/LTD intervals
  - configuration proposal/approval/apply workflow with history
  - append-only audit chain and retrieval APIs
  - communication-loss buffering/fail-closed behavior in event paths
- Remaining gaps:
  - production evidence completeness depends on executing operational drills and attaching artifacts per go-live gates.

## GLI-16 (Cashless Systems)
- Coverage status: strong core coverage.
- Implemented:
  - cashless ledger semantics, no-negative-balance controls
  - financial idempotency on state-changing operations
  - transfer denial/partial/unresolved state semantics
  - transaction statement/reporting support
  - EFT fraud lockout controls
- Remaining gaps:
  - advanced antifraud modeling (behavioral/network scoring) is not implemented.

## GLI-21 (Client-Server / Remote Access / Security Controls)
- Coverage status: strong server-side control coverage.
- Implemented:
  - remote admin boundary enforcement (trusted CIDR controls)
  - remote access activity logging and retrieval
  - strict production guardrails (DB + TLS + external keyset requirements)
  - JWT key rotation and external keyset loading hooks
- Remaining gaps:
  - full key custody lifecycle remains operational/deployment responsibility (KMS/HSM process controls and attestations).

## GLI-26 (Wireless)
- Coverage status: partial by design.
- Implemented:
  - server-side onboarding/hardening guidance (`docs/deployment/WIRELESS_ONBOARDING.md`)
  - authenticated/encrypted management expectations in deployment controls
- Remaining gaps:
  - wireless device/network hardware controls are out of initial product scope.

## GLI-28 (Player UI / System Windows)
- Coverage status: baseline/scaffold-plus-runtime.
- Implemented:
  - system-window open/close event capture and recall APIs
  - persistence/reportability of overlay event records
- Remaining gaps:
  - advanced client UX behavior enforcement (pre-notice decline flow, touch remap validation) depends on terminal/client implementation, not RGS alone.

## GLI-11 / GLI-12 / GLI-17 / GLI-18 / GLI-29
- Coverage status: mostly out-of-scope or indirect in current server-centric release.
- Notes:
  - physical device and cabinet controls are not part of this backend.
  - card shuffler mechanics are out of scope.
  - RNG/game math certification elements must be handled by game/device components and lab process.

## Cross-Cutting Gaps to Production-Ready Posture

1. Operational evidence execution
- Gap: all required drills/qualification artifacts are defined, but must be run in target environments for each release.
- Closure path: complete and attach evidence for Gate 1-10 in `docs/compliance/GO_LIVE_CHECKLIST.md`.

2. External key custody proof
- Gap: code supports file/command keyset integration, but production custody controls are process/infrastructure dependent.
- Closure path: run `make keyset-evidence`, attach rotation/runbook records, and complete key custody sign-off.

3. DB-backed qualification baselines
- Gap: soak/performance gates require environment-specific execution and threshold sign-off.
- Closure path: run `make soak-qual-db` and `make soak-qual-matrix` in each deployment profile and attach artifacts.

4. Promotions/UI advanced policy workflows
- Gap: baseline runtime APIs exist, advanced campaign policy orchestration is pending or must be explicitly deferred.
- Closure path: implement and test policy workflows, or record deferred scope acceptance in Gate 8 + Gate 10.

5. Package signing assurance depth
- Gap: current package signing verification is baseline; stronger asymmetric/certificate-chain controls may be required per jurisdiction/lab expectations.
- Closure path: implement and validate asymmetric signing/certificate-chain verification workflow; update compliance mapping/tests.

## Recommended Documentation/Process Next Steps

1. Keep `docs/compliance/REQUIREMENTS.md` updated with section-level references and concrete test evidence IDs per release.
2. Use `make gate10-evidence` to produce Gate 10 mapping artifacts and paste the generated checklist snippet into go-live review.
3. For every release candidate, attach:
- `make verify` output
- `make verify-evidence-strict` artifacts
- DR/failover/audit-chain/perf/soak/keyset evidence artifacts
4. Record explicit jurisdiction-specific deltas and acceptance decisions in the final submission packet.
