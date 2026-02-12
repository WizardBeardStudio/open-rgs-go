# Deployment Guide: Audit Chain Verification Evidence

This guide defines repeatable capture of audit-chain verification evidence using the
`AuditService/VerifyAuditChain` API.

## Prerequisites

- Running RGS endpoint with DB-backed mode enabled.
- Operator JWT token with permission to call audit APIs.
- `curl` and `jq` installed.

## Runbook

From `src/`:

```bash
RGS_AUDIT_BEARER_TOKEN="<operator-jwt>" \
RGS_AUDIT_PARTITION_DAY="2026-02-17" \
make audit-chain-evidence
```

Optional endpoint override:

```bash
RGS_AUDIT_VERIFY_URL="https://rgs.example.com/v1/audit/chain:verify" \
RGS_AUDIT_BEARER_TOKEN="<operator-jwt>" \
RGS_AUDIT_PARTITION_DAY="2026-02-17" \
make audit-chain-evidence
```

## Artifacts

Artifacts are written under `${RGS_AUDIT_CHAIN_WORKDIR:-/tmp/open-rgs-go-audit-chain}/<event id>/`:

- `request.json`
- `response.json`
- `summary.json`

`summary.json` includes:

- `result_code`
- `valid`
- `partition_day`
- pass/fail `result`

Use `summary.json` as release evidence for audit immutability verification.
