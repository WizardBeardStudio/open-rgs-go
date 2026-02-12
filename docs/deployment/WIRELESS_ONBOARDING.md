# Deployment Guide: Wireless Client Onboarding Constraints

This profile captures server-side constraints for clients connected over wireless links.

## Control Objectives
- Only registered/authenticated components can connect.
- Management/session traffic is authenticated and encrypted.
- Credentials and enrollment secrets are never stored in plaintext.

## Required Server Configuration
- Enable TLS: `RGS_TLS_ENABLED=true`.
- Require client certificates for management-plane access where possible:
  - `RGS_TLS_REQUIRE_CLIENT_CERT=true`
  - `RGS_TLS_CLIENT_CA_FILE=/path/to/ca.pem`
- Enable strict production policy:
  - `RGS_STRICT_PRODUCTION_MODE=true`
- Maintain trusted remote admin CIDRs:
  - `RGS_TRUSTED_CIDRS` scoped to ops networks only.

## Component Registration Expectations
- Every wireless interface element must be pre-registered with:
  - unique equipment/component id
  - certificate identity or token identity binding
  - approved protocol/version profile
- Unregistered components must be denied and logged.

## Secret Handling
- API-level credential provisioning uses hashed credential material where applicable.
- At-rest key material should be loaded from managed secret stores (KMS/HSM-backed) in production.

## Audit and Monitoring
- Record successful and denied connection attempts with source/destination metadata.
- Alert on denied-login spikes and lockout surges (see `METRICS_ALERTING.md`).
- Retain remote access activity history in durable storage in DB-backed mode.
