# AuthAndBalance Sample

This sample demonstrates:
- player login via `RgsClientBootstrap.Login(...)`
- authenticated balance retrieval via `RgsClient.GetBalanceAsync(...)`
- denial/error handling surfaced through bootstrap events

Script:
- `AuthAndBalanceSample.cs`

Usage:
1. Add `RgsClientBootstrap` to one GameObject and configure endpoint/actor defaults.
2. Add `AuthAndBalanceSample` to another GameObject.
3. Assign the bootstrap reference and player/account fields in Inspector.
4. Enter Play mode.
