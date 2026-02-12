# Deployment Guide: Download Package Signing

This guide defines baseline signature verification for download-library activation events.

## Runtime Configuration

- `RGS_DOWNLOAD_SIGNING_KEYS`
  - Comma-separated `kid:secret` entries.
  - Example: `pkg-k1:secret-v1,pkg-k2:secret-v2`

## Verification Behavior

- For `DOWNLOAD_ACTION_ACTIVATE`, the RGS requires:
  - `signer_kid`
  - `signature`
- Signature is validated as `HMAC-SHA256` over:
  - `library_path|checksum|version|action`
- Signature encoding: Base64 (raw, no padding).

## Rotation Workflow

1. Add new key to `RGS_DOWNLOAD_SIGNING_KEYS`.
2. Produce activation signatures using the new `signer_kid`.
3. Keep old key until all in-flight activation workflows are complete.
4. Remove old key from runtime config.

## Failure Semantics

- Missing activation signature fields return `INVALID`.
- Invalid signatures return `DENIED`.
- All denied activation attempts are audited.
