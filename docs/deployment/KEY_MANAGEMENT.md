# Deployment Guide: JWT Key Management and Rotation

This guide describes production key management for JWT signing/verification.

## Supported Runtime Sources

1. Environment keyset:
- `RGS_JWT_KEYSET` with `RGS_JWT_ACTIVE_KID`.

2. File-based keyset (recommended for KMS/HSM-integrated deployments):
- `RGS_JWT_KEYSET_FILE=/path/to/jwt-keyset.json`
- `RGS_JWT_KEYSET_REFRESH_INTERVAL=1m` (or lower for faster rotation convergence)

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

## Operational Notes

- In strict production mode (`RGS_STRICT_PRODUCTION_MODE=true`), default insecure secret is rejected unless external keyset config is provided.
- On reload failures, server keeps last-known-good keyset and logs refresh errors.
- Keep keyset file permissions restricted to runtime user (`0600`).
