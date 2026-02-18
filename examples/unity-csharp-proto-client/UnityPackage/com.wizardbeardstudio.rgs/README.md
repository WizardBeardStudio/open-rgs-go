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
  - `EventsClient.SubmitSignificantEventAsync`
  - `ReportingClient.GenerateReportAsync`
- `GrpcWebRgsTransport` implementation for `IRgsTransport` callers (gRPC-Web headers + HTTP transport).
- Parallel REST gateway wiring for the same methods when `RgsTransportMode.RestGateway` is selected.
  - WebGL runtime builds use `UnityWebRequest` for REST mode.
 - Token lifecycle support:
   - login
   - refresh token
   - logout
 - Package release assets:
    - `CHANGELOG.md`
    - `LICENSE.md`
    - `Documentation~/PACKAGE_RELEASE_CHECKLIST.md`
 - Importable sample scenes:
   - `Samples~/AuthAndBalance/AuthAndBalanceSampleScene.unity`
   - `Samples~/QuickStartSlot/QuickStartSlotSampleScene.unity`
   - editor wiring utility: `Tools > WizardBeard > RGS > ...`

Next implementation rounds should expand list/query APIs (events, meters, and report-run retrieval) and publish richer gameplay samples.
