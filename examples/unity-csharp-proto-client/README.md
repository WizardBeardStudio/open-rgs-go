# Unity C# Protobuf Client Example

This example generates C# protobuf + gRPC client stubs from `open-rgs-go` proto contracts.

## Prerequisites

- `.NET SDK` (8.x or newer recommended)
- `buf` CLI (for `google/api/*.proto` export)

## Generate Stubs

From repository `src/`:

```bash
./examples/unity-csharp-proto-client/scripts/sync_protos.sh
dotnet restore ./examples/unity-csharp-proto-client/rgs-unity-client.csproj
dotnet build -c Release ./examples/unity-csharp-proto-client/rgs-unity-client.csproj
```

Generated C# code is emitted by `Grpc.Tools` during build under:
- `examples/unity-csharp-proto-client/obj/...`

## Unity Integration

Copy resulting assemblies into Unity, for example:
- `Assets/Plugins/RGS/`

At minimum include:
- generated client library output (`rgs-unity-client.dll`)
- `Google.Protobuf.dll`
- gRPC runtime assemblies required by your chosen transport (`Grpc.Net.Client*`, `Grpc.Core.Api`, related deps)

For Unity transport guidance and request metadata requirements, see:
- `docs/integration/UNITY_CSHARP_PROTO_CLIENT.md`

Runtime Unity sample:
- `examples/unity-csharp-proto-client/UnitySample/README.md`
