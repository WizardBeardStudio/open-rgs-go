using System;
using System.Threading;
using UnityEngine;
using WizardBeardStudio.Rgs.Auth;
using WizardBeardStudio.Rgs.Core;
using WizardBeardStudio.Rgs.Diagnostics;
using WizardBeardStudio.Rgs.Models;
using WizardBeardStudio.Rgs.Services;

namespace WizardBeardStudio.Rgs
{
    public sealed class RgsClientBootstrap : MonoBehaviour
    {
        [SerializeField] private RgsClientConfig config = new RgsClientConfig();

        public event Action? OnInitialized;
        public event Action<string>? OnAuthenticated;
        public event Action? OnDisconnected;
        public event Action<string>? OnError;

        public RgsClient? Client { get; private set; }
        public RgsAuthService? AuthService { get; private set; }

        private IRgsLogger _logger = new UnityRgsLogger();

        private void Awake()
        {
            Initialize();
        }

        public void Initialize()
        {
            if (!config.Validate(out var error))
            {
                _logger.Error("RGS bootstrap validation failed: " + error);
                OnError?.Invoke(error);
                return;
            }

            var tokenStore = new InMemoryTokenStore();
            var identity = new IdentityClient();
            var auth = new RgsAuthService(identity, tokenStore);
            var sessions = new SessionsClient();
            var ledger = new LedgerClient();
            var wagering = new WageringClient();

            AuthService = auth;
            Client = new RgsClient(config, auth, sessions, ledger, wagering);
            _logger.Info("RGS client initialized");
            OnInitialized?.Invoke();
        }

        public async void Login(string playerId, string pin)
        {
            if (AuthService == null)
            {
                OnError?.Invoke("Auth service not initialized.");
                return;
            }

            try
            {
                var result = await AuthService.LoginPlayerAsync(playerId, pin, CancellationToken.None);
                if (!result.Success)
                {
                    OnError?.Invoke(result.DenialReason);
                    return;
                }
                OnAuthenticated?.Invoke(result.ActorId ?? playerId);
            }
            catch (Exception ex)
            {
                _logger.Error(ex.Message);
                OnError?.Invoke(ex.Message);
            }
        }

        public void Disconnect()
        {
            AuthService?.ClearSession();
            OnDisconnected?.Invoke();
        }
    }
}
