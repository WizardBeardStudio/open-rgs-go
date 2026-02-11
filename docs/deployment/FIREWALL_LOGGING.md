# Deployment Guide: Firewall and Logging Controls (Phase 5)

## Network Zones and Access Model
- Treat all access outside the trusted operations network as remote access.
- Restrict admin/control endpoints to trusted CIDR ranges only.
  - Admin paths include:
    - `/v1/config/*`
    - `/v1/reporting/*`
    - `/v1/audit/*` (when exposed)
- Default trusted CIDRs should be explicit and minimal.

## Runtime Controls
- `RGS_TRUSTED_CIDRS`
  - Comma-separated CIDRs used for remote admin boundary checks.
  - Example: `10.0.0.0/8,192.168.0.0/16,127.0.0.1/32,::1/128`
- Remote access guard behavior:
  - allow trusted source IPs to admin paths
  - deny untrusted source IPs with HTTP 403
  - log all admin access attempts (allowed/denied)

## Required Firewall/Audit Log Semantics
For each connection attempt (successful and unsuccessful), retain:
- timestamp
- source IP and source port
- destination host and destination port
- path and method
- decision (`allowed` or `denied`)
- denial reason when denied

For configuration-related changes, retain:
- proposer/approver/applier identities
- before/after values
- reason for change
- created/approved/applied timestamps

For download-library operations, retain:
- path
- version
- checksum
- action (`add`, `update`, `delete`, `activate`)
- actor identity and reason
- timestamp

## Fail-Closed Requirements
- If critical audit persistence is unavailable, state-changing operations must fail closed.
- If ingestion buffering reaches configured capacity, disable additional ingress for the affected boundary.
- If logs cannot be written in production deployment, treat the control plane as degraded and block risky admin operations.

## Operational Checks
- Verify trusted CIDR settings at startup and during deployment reviews.
- Verify denied admin attempts appear in activity/audit logs.
- Run chaos checks for:
  - lost communications / buffer exhaustion
  - audit-store unavailability (fail-closed)
  - untrusted network attempts to admin paths
