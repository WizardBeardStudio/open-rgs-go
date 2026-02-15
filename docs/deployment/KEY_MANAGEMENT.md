# Deployment Guide: JWT Key Management and Rotation

This guide describes production key management for JWT signing/verification.

## Supported Runtime Sources

1. Environment keyset:
- `RGS_JWT_KEYSET` with `RGS_JWT_ACTIVE_KID`.

2. File-based keyset (recommended for KMS/HSM-integrated deployments):
- `RGS_JWT_KEYSET_FILE=/path/to/jwt-keyset.json`
- `RGS_JWT_KEYSET_REFRESH_INTERVAL=1m` (or lower for faster rotation convergence)

3. Command-based keyset fetch (for KMS/HSM client wrappers):
- `RGS_JWT_KEYSET_COMMAND="kms-client get-jwt-keyset --format json"`
- `RGS_JWT_KEYSET_REFRESH_INTERVAL=1m`

File format:

```json
{
  "active_kid": "k2",
  "keys": {
    "k1": "old-signing-secret",
    "k2": "new-signing-secret"
  }
}
```

## Rotation Workflow

1. Provision new key material via KMS/HSM workflow.
2. Update keyset file to include both old and new keys.
3. Set `active_kid` to new key.
4. Allow refresh interval to elapse; server reloads keyset in-process.
5. Wait for old access tokens to expire.
6. Remove old key from keyset file.

## Rotation Evidence Workflow

Capture auditable artifacts for each rotation event:

```bash
RGS_JWT_KEYSET_COMMAND="kms-client get-jwt-keyset --format json" \
RGS_KEYSET_EVENT_ID=rotation-20260212-a \
make keyset-evidence
```

Or from file source:

```bash
RGS_JWT_KEYSET_FILE=/etc/rgs/jwt-keyset.json \
RGS_KEYSET_EVENT_ID=rotation-20260212-a \
make keyset-evidence
```

Compare against a previous event summary to identify active-kid transitions:

```bash
RGS_JWT_KEYSET_FILE=/etc/rgs/jwt-keyset.json \
RGS_KEYSET_PREVIOUS_SUMMARY_FILE=/tmp/open-rgs-go-keyset/rotation-prev/summary.json \
make keyset-evidence
```

Artifacts are written under `${RGS_KEYSET_WORKDIR:-/tmp/open-rgs-go-keyset}/<event_id>/`:
- `keyset.json`
- `summary.json`
- `fingerprint.sha256`

## Operational Notes

- In strict production mode (`RGS_STRICT_PRODUCTION_MODE=true`), default insecure secret is rejected unless external keyset config is provided.
- `RGS_STRICT_EXTERNAL_JWT_KEYSET` defaults to `true` when strict production mode is enabled; this requires `RGS_JWT_KEYSET_FILE` or `RGS_JWT_KEYSET_COMMAND` and prevents inline key material usage.
- On reload failures, server keeps last-known-good keyset and logs refresh errors.
- Keep keyset file permissions restricted to runtime user (`0600`).
- `scripts/keyset_rotation_evidence.sh` requires `jq` for summary extraction and validation.

## Verification Evidence Attestation Keys

`make verify-evidence-strict` signs `attestation.json` and validates signatures during `verify-summary`.
Strict/CI mode requires `ed25519`.

Required runtime variables for strict/CI evidence:
- `RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID` (for example `ci-active`)
- `RGS_VERIFY_EVIDENCE_ATTESTATION_ALG=ed25519`
- One of:
  - `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY` for single-key mode
  - `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEYS` for rotation windows in key-ring mode, format:
    - `active:<base64_private_or_seed>,previous:<base64_private_or_seed>`
- One of:
  - `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEY` for single-key verification
  - `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEYS` for rotation windows in key-ring mode, format:
    - `active:<base64_public>,previous:<base64_public>`
- `RGS_VERIFY_EVIDENCE_ENFORCE_ATTESTATION_KEY=true` (enabled by `make verify-evidence-strict`)

Compatibility mode:
- `hmac-sha256` can still be used for non-strict local verification compatibility.
- HMAC mode is scheduled for retirement at API freeze.

### Secret Source Patterns

1. GitHub Actions secrets:
- `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEY` stored in repository/org secrets
- `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEYS` stored in repository/org secrets
- `RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID` set in workflow (`ci-active`)

2. Vault injection:
- sidecar/agent renders env vars at runtime
- runner exports `RGS_VERIFY_EVIDENCE_ATTESTATION_KEYS` with active+previous entries

3. KMS-wrapped secret material:
- decrypt key material at runtime in CI bootstrap
- export only process-scoped env vars for the job duration

Do not commit attestation keys into repository files or workflow YAML literals.

### Rotation Runbook (Attestation Keys)

1. Generate new key material in secret manager (Vault/KMS/CI secrets backend).
2. Assign a new key id (for example `ci-2026-02`).
3. During overlap window, publish key ring:
   - `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PRIVATE_KEYS="ci-2026-02:<new_private>,ci-2026-01:<old_private>"`
   - `RGS_VERIFY_EVIDENCE_ATTESTATION_ED25519_PUBLIC_KEYS="ci-2026-02:<new_public>,ci-2026-01:<old_public>"`
4. Set signing key id:
   - `RGS_VERIFY_EVIDENCE_ATTESTATION_KEY_ID=ci-2026-02`
5. Run strict verification:
   - `make verify-evidence-strict`
   - `make verify-summary`
6. After retention window for old evidence verification, remove old key id from key ring.
7. Record rotation event id and evidence artifact path in operations log.
