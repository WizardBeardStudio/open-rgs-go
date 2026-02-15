using UnityEngine;

namespace WizardBeardStudio.Rgs.Samples
{
    public sealed class QuickStartSlotSample : MonoBehaviour
    {
        [SerializeField] private RgsClientBootstrap bootstrap = null!;
        [SerializeField] private string playerId = "player-1";
        [SerializeField] private string playerPin = "1234";
        [SerializeField] private string gameId = "slot-default";
        [SerializeField] private long wagerMinor = 100;
        [SerializeField] private long payoutMinor = 150;
        [SerializeField] private string currency = "USD";

        private void Start()
        {
            if (bootstrap == null)
            {
                Debug.LogError("Assign RgsClientBootstrap in Inspector.");
                return;
            }
            bootstrap.OnAuthenticated += RunFlow;
            bootstrap.OnError += HandleError;
            bootstrap.Login(playerId, playerPin);
        }

        private async void RunFlow(string actorId)
        {
            if (bootstrap.Client == null)
            {
                Debug.LogError("RGS client unavailable.");
                return;
            }

            var session = await bootstrap.Client.StartSessionAsync(string.Empty, default);
            if (!session.Success)
            {
                Debug.LogWarning($"StartSession denied: {session.ResultCode} {session.DenialReason}");
                return;
            }

            var wager = await bootstrap.Client.PlaceWagerAsync(gameId, wagerMinor, currency, null, default);
            if (!wager.Success)
            {
                Debug.LogWarning($"PlaceWager denied: {wager.ResultCode} {wager.DenialReason}");
                await bootstrap.Client.EndSessionAsync(session.SessionId, default);
                return;
            }

            var settle = await bootstrap.Client.SettleWagerAsync(wager.WagerId, payoutMinor, currency, default);
            if (!settle.Success)
            {
                Debug.LogWarning($"SettleWager denied: {settle.ResultCode} {settle.DenialReason}");
            }

            var end = await bootstrap.Client.EndSessionAsync(session.SessionId, default);
            if (!end.Success)
            {
                Debug.LogWarning($"EndSession denied: {end.ResultCode} {end.DenialReason}");
            }

            Debug.Log($"QuickStartSlot flow complete for actor={actorId}, session={session.SessionId}, wager={wager.WagerId}, settle_status={settle.ResultCode}");
        }

        private static void HandleError(string message)
        {
            Debug.LogError("RGS quickstart error: " + message);
        }

        private void OnDestroy()
        {
            if (bootstrap != null)
            {
                bootstrap.OnAuthenticated -= RunFlow;
                bootstrap.OnError -= HandleError;
            }
        }
    }
}
