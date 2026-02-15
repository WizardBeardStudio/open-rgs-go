using System;
using System.Net.Http;
using System.Threading;
using Grpc.Net.Client;
using Grpc.Net.Client.Web;
using Rgs.V1;
using UnityEngine;
using WizardBeardStudio.Rgs.Auth;
using WizardBeardStudio.Rgs.Core;
using WizardBeardStudio.Rgs.Diagnostics;
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

        private readonly IRgsLogger _logger = new UnityRgsLogger();
        private GrpcChannel? _channel;
        private InMemoryTokenStore? _tokenStore;

        private void Awake()
        {
            Initialize();
        }

        private void OnDestroy()
        {
            _channel?.Dispose();
            _channel = null;
        }

        public void Initialize()
        {
            if (!config.Validate(out var error))
            {
                _logger.Error("RGS bootstrap validation failed: " + error);
                OnError?.Invoke(error);
                return;
            }

            _tokenStore = new InMemoryTokenStore();
            _channel = CreateChannel(config);

            var identityStub = new IdentityService.IdentityServiceClient(_channel);
            var ledgerStub = new LedgerService.LedgerServiceClient(_channel);

            var identity = new IdentityClient(identityStub, config.deviceId, config.userAgent, config.geo);
            var auth = new RgsAuthService(identity, _tokenStore);
            var sessions = new SessionsClient();
            var ledger = new LedgerClient(
                ledgerStub,
                () => _tokenStore.AccessToken,
                config.playerId,
                config.deviceId,
                config.userAgent,
                config.geo);
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

        private GrpcChannel CreateChannel(RgsClientConfig cfg)
        {
            if (cfg.transportMode == RgsTransportMode.RestGateway)
            {
                _logger.Warn("REST transport mode selected in config; bootstrap currently uses gRPC-Web generated clients for runtime sample paths.");
            }

            var handler = new GrpcWebHandler(GrpcWebMode.GrpcWebText, new HttpClientHandler());
            return GrpcChannel.ForAddress(cfg.baseUrl, new GrpcChannelOptions
            {
                HttpHandler = handler
            });
        }
    }
}
