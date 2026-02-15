# Quick Start

1. Add `RgsClientBootstrap` to an active GameObject.
2. Set base URL, transport mode (`GrpcWeb` or `RestGateway`), player ID, and device metadata.
3. Trigger `Login(playerId, pin)` from your UI flow.
4. Use `Client` facade for session/wagering/ledger calls.
