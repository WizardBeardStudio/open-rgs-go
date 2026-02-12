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
