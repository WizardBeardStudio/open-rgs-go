using System;
using System.Collections.Generic;
using System.Reflection;
using System.Threading;
using System.Threading.Tasks;
using NUnit.Framework;
using UnityEngine;
using WizardBeardStudio.Rgs.Auth;
using WizardBeardStudio.Rgs.Core;
using WizardBeardStudio.Rgs.Services;

namespace WizardBeardStudio.Rgs.Tests.Runtime
{
    public sealed class RuntimeSmokeTests
    {
        [Test]
        public void BootstrapInitialize_WithInvalidConfig_RaisesErrorEvent()
        {
            var go = new GameObject("rgs-bootstrap-test");
            try
            {
                var bootstrap = go.AddComponent<RgsClientBootstrap>();
                var configField = typeof(RgsClientBootstrap).GetField("config", BindingFlags.NonPublic | BindingFlags.Instance);
                Assert.That(configField, Is.Not.Null);

                configField!.SetValue(bootstrap, new RgsClientConfig
                {
                    baseUrl = string.Empty,
                    playerId = "player-1",
                    defaultGameId = "slot-1"
                });

                string? errorMessage = null;
                bootstrap.OnError += msg => errorMessage = msg;

                bootstrap.Initialize();

                Assert.That(errorMessage, Is.Not.Null.And.Not.Empty);
            }
            finally
            {
                UnityEngine.Object.DestroyImmediate(go);
            }
        }

        [Test]
        public async Task LoginAndBalance_WithMockTransport_Succeeds()
        {
            var transport = new FakeTransport();
            var tokenStore = new InMemoryTokenStore();
            var identity = new IdentityClient(transport, "dev-1", "ua", string.Empty);
            var auth = new RgsAuthService(identity, tokenStore);
            var ledger = new LedgerClient(transport, () => tokenStore.AccessToken, "player-1", "dev-1", "ua", string.Empty);

            var login = await auth.LoginPlayerAsync("player-1", "1234", CancellationToken.None);
            if (login.Success)
            {
                tokenStore.Save(login.AccessToken ?? string.Empty, login.RefreshToken ?? string.Empty, login.ActorId ?? "player-1");
            }
            var balance = await ledger.GetBalanceAsync("acct-player-1", CancellationToken.None);

            Assert.That(login.Success, Is.True);
            Assert.That(balance.Success, Is.True);
            Assert.That(balance.AvailableMinor, Is.EqualTo(2500));
            Assert.That(balance.Currency, Is.EqualTo("USD"));
        }

        private sealed class FakeTransport : IRgsTransport
        {
            public Task<string> PostJsonAsync(string path, string jsonBody, IDictionary<string, string>? headers, CancellationToken cancellationToken)
            {
                if (path == "/v1/identity/login")
                {
                    return Task.FromResult("{" +
                                           "\"meta\":{" +
                                           "\"requestId\":\"req-login\"," +
                                           "\"resultCode\":\"RESULT_CODE_OK\"," +
                                           "\"denialReason\":\"\"," +
                                           "\"serverTime\":\"2026-02-15T00:00:00Z\"}," +
                                           "\"token\":{" +
                                           "\"accessToken\":\"a\"," +
                                           "\"refreshToken\":\"r\"," +
                                           "\"actor\":{\"actorId\":\"player-1\"}}}");
                }

                return Task.FromResult("{" +
                                       "\"meta\":{" +
                                       "\"requestId\":\"req-x\"," +
                                       "\"resultCode\":\"RESULT_CODE_OK\"," +
                                       "\"denialReason\":\"\"," +
                                       "\"serverTime\":\"2026-02-15T00:00:00Z\"}}" );
            }

            public Task<string> GetJsonAsync(string path, IDictionary<string, string>? headers, CancellationToken cancellationToken)
            {
                if (path == "/v1/ledger/accounts/acct-player-1/balance")
                {
                    return Task.FromResult("{" +
                                           "\"meta\":{" +
                                           "\"requestId\":\"req-balance\"," +
                                           "\"resultCode\":\"RESULT_CODE_OK\"," +
                                           "\"denialReason\":\"\"," +
                                           "\"serverTime\":\"2026-02-15T00:00:00Z\"}," +
                                           "\"availableBalance\":{\"amountMinor\":2500,\"currency\":\"USD\"}," +
                                           "\"pendingBalance\":{\"amountMinor\":0,\"currency\":\"USD\"}}" );
                }

                return Task.FromResult("{" +
                                       "\"meta\":{" +
                                       "\"requestId\":\"req-g\"," +
                                       "\"resultCode\":\"RESULT_CODE_ERROR\"," +
                                       "\"denialReason\":\"not found\"," +
                                       "\"serverTime\":\"2026-02-15T00:00:00Z\"}}" );
            }
        }
    }
}
