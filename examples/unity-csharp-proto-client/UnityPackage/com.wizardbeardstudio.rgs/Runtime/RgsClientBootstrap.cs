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
        private IDisposable? _restTransportDisposable;
        private InMemoryTokenStore? _tokenStore;

        private void Awake()
        {
            Initialize();
        }

        private void OnDestroy()
        {
            _channel?.Dispose();
            _channel = null;
            _restTransportDisposable?.Dispose();
            _restTransportDisposable = null;
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

            IdentityClient identity;
            SessionsClient sessions;
            LedgerClient ledger;
            WageringClient wagering;
            EventsClient events;
            ReportingClient reporting;

            if (config.transportMode == RgsTransportMode.RestGateway)
            {
                var rest = new RestGatewayRgsTransport(config.baseUrl, config.requestTimeoutSeconds);
                _restTransportDisposable = rest;

                identity = new IdentityClient(rest, config.deviceId, config.userAgent, config.geo);
                sessions = new SessionsClient(rest, () => _tokenStore.AccessToken, config.playerId, config.deviceId, config.userAgent, config.geo);
                ledger = new LedgerClient(rest, () => _tokenStore.AccessToken, config.playerId, config.deviceId, config.userAgent, config.geo);
                wagering = new WageringClient(rest, () => _tokenStore.AccessToken, config.playerId, config.deviceId, config.userAgent, config.geo);
                events = new EventsClient(rest, () => _tokenStore.AccessToken, config.playerId, config.deviceId, config.userAgent, config.geo, config.equipmentId);
                reporting = new ReportingClient(rest, () => _tokenStore.AccessToken, config.playerId, config.deviceId, config.userAgent, config.geo, config.operatorId);
                _logger.Info("RGS client configured for REST gateway transport");
            }
            else
            {
                _channel = CreateChannel(config);

                var identityStub = new IdentityService.IdentityServiceClient(_channel);
                var ledgerStub = new LedgerService.LedgerServiceClient(_channel);
                var sessionsStub = new SessionsService.SessionsServiceClient(_channel);
                var wageringStub = new WageringService.WageringServiceClient(_channel);
                var eventsStub = new EventsService.EventsServiceClient(_channel);
                var reportingStub = new ReportingService.ReportingServiceClient(_channel);

                identity = new IdentityClient(identityStub, config.deviceId, config.userAgent, config.geo);
                sessions = new SessionsClient(sessionsStub, () => _tokenStore.AccessToken, config.playerId, config.deviceId, config.userAgent, config.geo);
                ledger = new LedgerClient(ledgerStub, () => _tokenStore.AccessToken, config.playerId, config.deviceId, config.userAgent, config.geo);
                wagering = new WageringClient(wageringStub, () => _tokenStore.AccessToken, config.playerId, config.deviceId, config.userAgent, config.geo);
                events = new EventsClient(eventsStub, () => _tokenStore.AccessToken, config.playerId, config.deviceId, config.userAgent, config.geo, config.equipmentId);
                reporting = new ReportingClient(reportingStub, () => _tokenStore.AccessToken, config.playerId, config.deviceId, config.userAgent, config.geo, config.operatorId);
                _logger.Info("RGS client configured for gRPC-Web transport");
            }

            var auth = new RgsAuthService(identity, _tokenStore);
            AuthService = auth;
            Client = new RgsClient(config, auth, sessions, ledger, wagering, events, reporting);
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

        public async void LogoutAndDisconnect()
        {
            if (AuthService != null)
            {
                await AuthService.LogoutAsync(CancellationToken.None);
            }
            OnDisconnected?.Invoke();
        }

        private static GrpcChannel CreateChannel(RgsClientConfig cfg)
        {
            var handler = new GrpcWebHandler(GrpcWebMode.GrpcWebText, new HttpClientHandler());
            return GrpcChannel.ForAddress(cfg.baseUrl, new GrpcChannelOptions
            {
                HttpHandler = handler
            });
        }
    }
}
