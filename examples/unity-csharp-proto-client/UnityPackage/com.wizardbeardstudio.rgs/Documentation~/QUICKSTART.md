# Quick Start

1. Add `RgsClientBootstrap` to an active GameObject.
2. Set base URL, transport mode (`GrpcWeb` or `RestGateway`), player ID, and device metadata.
3. For WebGL builds, prefer `RestGateway` mode (uses `UnityWebRequest` in runtime builds).
4. Trigger `Login(playerId, pin)` from your UI flow.
5. Use `Client` facade for session/wagering/ledger calls.
