# open-rgs-go (Phase 0 Scaffold)

GLI-aligned Remote Gaming System backend scaffold.

## Quick start

```bash
go test ./...
go run ./cmd/rgsd
```

## Endpoints
- HTTP: `GET /healthz`, `GET /v1/system/status`
- gRPC: standard gRPC health service

## Phase 0 Contents
- Go module and baseline service layout
- Protobuf source-of-truth scaffold under `api/proto`
- Buf config for lint + code generation
- Audit model with hash chaining and append-only migration
- Basic JWT auth verifier and middleware
- CI workflow for tests and proto checks
- Compliance artifacts scaffold under `docs/compliance`
