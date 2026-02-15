# QuickStartSlot Sample

This sample demonstrates:
- login -> start session -> place wager -> settle wager -> end session
- idempotency key auto-generation for financial requests
- denial/error handling in each step

Script:
- `QuickStartSlotSample.cs`

Usage:
1. Add `RgsClientBootstrap` to one GameObject and configure endpoint/player defaults.
2. Add `QuickStartSlotSample` to another GameObject.
3. Assign bootstrap reference and set `gameId`, `wagerMinor`, and `payoutMinor`.
4. Enter Play mode.
