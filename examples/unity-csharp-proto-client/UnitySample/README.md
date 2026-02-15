# Unity Runtime Sample

This folder contains a minimal Unity-facing C# script that demonstrates:
- `IdentityService.Login`
- authenticated `LedgerService.GetBalance`

## Prerequisites

1. Build generated client assemblies first:

```bash
./examples/unity-csharp-proto-client/scripts/sync_protos.sh
dotnet restore ./examples/unity-csharp-proto-client/rgs-unity-client.csproj
dotnet build -c Release ./examples/unity-csharp-proto-client/rgs-unity-client.csproj
```

2. Copy assemblies into your Unity project, for example:
- `Assets/Plugins/RGS/rgs-unity-client.dll`
- `Assets/Plugins/RGS/Google.Protobuf.dll`
- gRPC runtime dependencies used by your transport path (`Grpc.Net.Client*`, `Grpc.Core.Api`, related dependencies)

3. Copy script:
- `UnitySample/RgsClientBehaviour.cs` -> `Assets/Scripts/RgsClientBehaviour.cs`

## Configure in Unity Inspector

- `baseUrl`: gRPC-Web endpoint (for example `https://rgs-gateway.example.com`)
- `playerId`
- `playerPin`
- `accountId`
- optional `deviceId`

## Runtime Notes

- Script performs login on `Start()`, then requests balance.
- `meta.actor` is set to the logged-in player actor.
- For state-changing calls, always set a unique `meta.idempotency_key`.
- Handle denied/invalid responses from `response.meta.result_code` and `response.meta.denial_reason`.
