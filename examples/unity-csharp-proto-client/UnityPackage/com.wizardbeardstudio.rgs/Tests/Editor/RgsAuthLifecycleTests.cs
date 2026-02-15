using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;
using NUnit.Framework;
using WizardBeardStudio.Rgs.Auth;
using WizardBeardStudio.Rgs.Core;
using WizardBeardStudio.Rgs.Services;

namespace WizardBeardStudio.Rgs.Tests.Editor
{
    public sealed class RgsAuthLifecycleTests
    {
        [Test]
        public async Task RefreshToken_WhenTokenExists_UpdatesStoredTokens()
        {
            var transport = new FakeAuthTransport();
            var tokenStore = new InMemoryTokenStore();
            var identity = new IdentityClient(transport, "dev-1", "ua", string.Empty);
            var auth = new RgsAuthService(identity, tokenStore);

            var login = await auth.LoginPlayerAsync("player-1", "1234", CancellationToken.None);
            Assert.That(login.Success, Is.True);

            var refresh = await auth.RefreshTokenAsync(CancellationToken.None);

            Assert.That(refresh.Success, Is.True);
            Assert.That(tokenStore.AccessToken, Is.EqualTo("access-2"));
            Assert.That(tokenStore.RefreshToken, Is.EqualTo("refresh-2"));
            Assert.That(tokenStore.ActorId, Is.EqualTo("player-1"));
        }

        [Test]
        public async Task Logout_ClearsTokenStore()
        {
            var transport = new FakeAuthTransport();
            var tokenStore = new InMemoryTokenStore();
            var identity = new IdentityClient(transport, "dev-1", "ua", string.Empty);
            var auth = new RgsAuthService(identity, tokenStore);

            await auth.LoginPlayerAsync("player-1", "1234", CancellationToken.None);
            var logout = await auth.LogoutAsync(CancellationToken.None);

            Assert.That(logout.Success, Is.True);
            Assert.That(tokenStore.AccessToken, Is.Null);
            Assert.That(tokenStore.RefreshToken, Is.Null);
            Assert.That(tokenStore.ActorId, Is.Null);
        }

        private sealed class FakeAuthTransport : IRgsTransport
        {
            public Task<string> PostJsonAsync(string path, string jsonBody, IDictionary<string, string>? headers, CancellationToken cancellationToken)
            {
                if (path == "/v1/identity/login")
                {
                    return Task.FromResult("{" +
                                           "\"meta\":{\"requestId\":\"req-login\",\"resultCode\":\"RESULT_CODE_OK\",\"denialReason\":\"\",\"serverTime\":\"2026-02-15T00:00:00Z\"}," +
                                           "\"token\":{\"accessToken\":\"access-1\",\"refreshToken\":\"refresh-1\",\"actor\":{\"actorId\":\"player-1\"}}}");
                }
                if (path == "/v1/identity/refresh")
                {
                    return Task.FromResult("{" +
                                           "\"meta\":{\"requestId\":\"req-refresh\",\"resultCode\":\"RESULT_CODE_OK\",\"denialReason\":\"\",\"serverTime\":\"2026-02-15T00:00:00Z\"}," +
                                           "\"token\":{\"accessToken\":\"access-2\",\"refreshToken\":\"refresh-2\",\"actor\":{\"actorId\":\"player-1\"}}}");
                }
                if (path == "/v1/identity/logout")
                {
                    return Task.FromResult("{" +
                                           "\"meta\":{\"requestId\":\"req-logout\",\"resultCode\":\"RESULT_CODE_OK\",\"denialReason\":\"\",\"serverTime\":\"2026-02-15T00:00:00Z\"}}" );
                }
                return Task.FromResult("{\"meta\":{\"requestId\":\"req-x\",\"resultCode\":\"RESULT_CODE_ERROR\",\"denialReason\":\"unsupported\",\"serverTime\":\"2026-02-15T00:00:00Z\"}}" );
            }

            public Task<string> GetJsonAsync(string path, IDictionary<string, string>? headers, CancellationToken cancellationToken)
            {
                return Task.FromResult("{\"meta\":{\"requestId\":\"req-g\",\"resultCode\":\"RESULT_CODE_OK\",\"denialReason\":\"\",\"serverTime\":\"2026-02-15T00:00:00Z\"}}" );
            }
        }
    }
}
