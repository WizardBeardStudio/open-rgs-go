# Threat Model (Phase 0 Scaffold)

## In Scope
- External API boundary for gRPC and REST
- Operator/player JWT authentication
- Optional mTLS between clients and RGS
- Audit log integrity chain and append-only constraints

## Initial Threats
- Unauthorized API access via forged/replayed credentials
- Privilege escalation across actor types
- Tampering with audit records
- Plaintext transport for admin pathways

## Initial Mitigations
- JWT verification middleware with strict claim requirements
- Separate actor typing (`player`, `operator`, `service`) for policy enforcement
- mTLS option at transport layer for registered clients
- Hash chaining + append-only DB triggers for audit integrity

## Follow-up in Phase 1+
- Key rotation strategy and HSM/KMS envelope support
- Rate limiting / lockout behavior
- Role-based authorization policy matrix
- Remote access session recording semantics
