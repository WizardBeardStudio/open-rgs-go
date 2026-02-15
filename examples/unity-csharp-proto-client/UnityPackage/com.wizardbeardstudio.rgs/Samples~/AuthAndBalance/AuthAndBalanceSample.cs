using UnityEngine;

namespace WizardBeardStudio.Rgs.Samples
{
    public sealed class AuthAndBalanceSample : MonoBehaviour
    {
        [SerializeField] private RgsClientBootstrap bootstrap = null!;
        [SerializeField] private string playerId = "player-1";
        [SerializeField] private string playerPin = "1234";
        [SerializeField] private string accountId = "acct-player-1";

        private async void Start()
        {
            if (bootstrap == null)
            {
                Debug.LogError("Assign RgsClientBootstrap in Inspector.");
                return;
            }

            bootstrap.OnError += HandleError;
            bootstrap.OnAuthenticated += HandleAuthenticated;
            bootstrap.Login(playerId, playerPin);

            while (bootstrap.Client == null)
            {
                await System.Threading.Tasks.Task.Yield();
            }
        }

        private async void HandleAuthenticated(string actorId)
        {
            if (bootstrap.Client == null)
            {
                Debug.LogError("RGS client is not initialized.");
                return;
            }

            var balance = await bootstrap.Client.GetBalanceAsync(accountId, default);
            if (!balance.Success)
            {
                Debug.LogWarning($"Balance denied for {actorId}: {balance.ResultCode} {balance.DenialReason}");
                return;
            }

            Debug.Log($"Authenticated actor={actorId}, balance={balance.AvailableMinor} {balance.Currency} minor units");
        }

        private static void HandleError(string message)
        {
            Debug.LogError("RGS sample error: " + message);
        }

        private void OnDestroy()
        {
            if (bootstrap != null)
            {
                bootstrap.OnError -= HandleError;
                bootstrap.OnAuthenticated -= HandleAuthenticated;
            }
        }
    }
}
