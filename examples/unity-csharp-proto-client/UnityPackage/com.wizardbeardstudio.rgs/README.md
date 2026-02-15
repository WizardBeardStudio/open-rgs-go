# WizardBeard RGS Unity SDK (Scaffold)

This is the initial Unity SDK scaffold for integrating with open-rgs-go.

Includes:
- Runtime bootstrap/config and service facade signatures
- Token store and metadata helpers
- Transport abstractions
- Editor inspector helper
- Documentation and sample placeholders
- Initial gRPC wiring for:
  - `IdentityClient.LoginPlayerAsync`
  - `LedgerClient.GetBalanceAsync`
  - `SessionsClient.StartSessionAsync` / `EndSessionAsync`
  - `WageringClient.PlaceWagerAsync` / `SettleWagerAsync`

Next implementation rounds should wire service classes to generated protobuf clients and publish importable sample scenes.
