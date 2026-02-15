# Unity + C# Protobuf Client Guide (RGS)

This guide shows one practical way to build a Unity-compatible C# client for `open-rgs-go` APIs.

Runnable example project:
- `examples/unity-csharp-proto-client/`
- Unity runtime sample script:
  - `examples/unity-csharp-proto-client/UnitySample/RgsClientBehaviour.cs`

## Scope

- Proto source of truth: `api/proto/rgs/v1/*.proto`
- Transport options:
  - gRPC (preferred where platform/network supports HTTP/2 or gRPC-Web)
  - REST JSON via grpc-gateway (fallback/easier Unity integration path)

## 1) Generate C# Client Types and gRPC Stubs

Use a separate .NET class library for generated code, then reference the DLLs from Unity.

Example layout:

```text
tools/rgs-unity-client/
  rgs-unity-client.csproj
  Protos/rgs/v1/*.proto
  Protos/google/api/*.proto
```

Copy proto sources:
- `api/proto/rgs/v1/*.proto` -> `Protos/rgs/v1/`
- `google/api/*.proto` (from googleapis) -> `Protos/google/api/`

Example `rgs-unity-client.csproj`:

```xml
<Project Sdk="Microsoft.NET.Sdk">
  <PropertyGroup>
    <TargetFramework>netstandard2.1</TargetFramework>
    <LangVersion>latest</LangVersion>
  </PropertyGroup>

  <ItemGroup>
    <PackageReference Include="Google.Protobuf" Version="3.27.2" />
    <PackageReference Include="Grpc.Net.Client" Version="2.66.0" />
    <PackageReference Include="Grpc.Net.Client.Web" Version="2.66.0" />
    <PackageReference Include="Grpc.Tools" Version="2.66.0" PrivateAssets="All" />
  </ItemGroup>

  <ItemGroup>
    <Protobuf Include="Protos/rgs/v1/*.proto" GrpcServices="Client" ProtoRoot="Protos" />
    <Protobuf Include="Protos/google/api/*.proto" GrpcServices="None" ProtoRoot="Protos" />
  </ItemGroup>
</Project>
```

Build:

```bash
dotnet restore
dotnet build -c Release
```

Then copy output DLLs into Unity (for example `Assets/Plugins/RGS/`):
- generated client assembly
- `Google.Protobuf.dll`
- gRPC runtime dependencies used by your chosen transport

## 2) Unity Transport Choice

## Option A: gRPC-Web (recommended for Unity portability)

Use `Grpc.Net.Client.Web` with a gRPC-Web compatible endpoint/proxy in front of RGS.

High-level:
1. Expose RGS through a gRPC-Web proxy (Envoy/grpcwebproxy).
2. In Unity C#, create a `GrpcWebHandler`.
3. Create service clients from generated stubs.

## Option B: REST JSON through Gateway

RGS already exposes REST via grpc-gateway on HTTP (`/v1/...` routes in proto annotations).
For Unity teams that prefer lower integration risk, call REST endpoints directly and keep protobuf contracts as schema reference.

## 3) Authentication and Request Metadata

For protected APIs:
- Set `Authorization: Bearer <access_token>` header.
- Keep `meta.actor` aligned with token actor claims. Mismatch is denied and audited.
- Include:
  - `meta.request_id` (UUID)
  - `meta.idempotency_key` for state-changing financial requests
  - `meta.source` fields when available

The canonical metadata structures are in:
- `api/proto/rgs/v1/common.proto`

## 4) Minimal C# Example (gRPC client call)

```csharp
using Grpc.Core;
using Grpc.Net.Client;
using Grpc.Net.Client.Web;
using Rgs.V1;

public async Task<GetBalanceResponse> GetBalanceAsync(string baseUrl, string bearerToken, string accountId)
{
    var handler = new GrpcWebHandler(GrpcWebMode.GrpcWebText, new HttpClientHandler());
    using var channel = GrpcChannel.ForAddress(baseUrl, new GrpcChannelOptions { HttpHandler = handler });
    var client = new LedgerService.LedgerServiceClient(channel);

    var headers = new Metadata
    {
        { "authorization", $"Bearer {bearerToken}" }
    };

    var req = new GetBalanceRequest
    {
        Meta = new RequestMeta
        {
            RequestId = Guid.NewGuid().ToString(),
            Actor = new Actor
            {
                ActorId = "player-123",
                ActorType = ActorType.Player
            },
            Source = new Source
            {
                DeviceId = "unity-slot-client-01",
                UserAgent = "unity-client"
            }
        },
        AccountId = accountId
    };

    return await client.GetBalanceAsync(req, headers);
}
```

## 5) Unity Slot Client Integration Pattern

1. Authenticate with `IdentityService.Login` to obtain access/refresh tokens.
2. Use access token for gameplay session calls:
- `SessionsService.StartSession`
- `LedgerService.GetBalance`
- `WageringService.PlaceWager`
- `WageringService.SettleWager` (or upstream outcome path)
3. On token expiry, call `IdentityService.RefreshToken`.
4. On disconnect/exit, call `SessionsService.EndSession` and `IdentityService.Logout`.

## 6) Operational/Compliance Client Requirements

For GLI-aligned client behavior:
- Always send `request_id`; never reuse idempotency keys across distinct financial operations.
- Handle denial responses explicitly and display safe failure reasons.
- Preserve device/session identity consistency across request metadata.
- Do not cache or store credential material insecurely in client code/assets.

## 7) Troubleshooting

1. `UNAUTHENTICATED` / `PERMISSION_DENIED`
- Verify bearer token validity and `meta.actor` alignment with token actor.

2. gRPC transport errors from Unity
- Use gRPC-Web proxy path, or switch to REST gateway endpoints for unsupported platforms.

3. Proto compile issues for `google/api/annotations.proto`
- Ensure `google/api/*.proto` exists under the configured `ProtoRoot`.

4. Idempotency-related denials on financial calls
- Ensure each new financial operation has a new unique idempotency key.
