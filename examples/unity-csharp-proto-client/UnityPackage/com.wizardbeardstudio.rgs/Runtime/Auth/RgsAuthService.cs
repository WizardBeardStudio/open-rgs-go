using System;
using System.Threading;
using System.Threading.Tasks;
using WizardBeardStudio.Rgs.Models;
using WizardBeardStudio.Rgs.Services;

namespace WizardBeardStudio.Rgs.Auth
{
    public sealed class RgsAuthService
    {
        private readonly IdentityClient _identityClient;
        private readonly ITokenStore _tokenStore;

        public RgsAuthService(IdentityClient identityClient, ITokenStore tokenStore)
        {
            _identityClient = identityClient;
            _tokenStore = tokenStore;
        }

        public async Task<LoginResult> LoginPlayerAsync(string playerId, string pin, CancellationToken cancellationToken)
        {
            var result = await _identityClient.LoginPlayerAsync(playerId, pin, cancellationToken);
            if (result.Success)
            {
                _tokenStore.Save(result.AccessToken ?? string.Empty, result.RefreshToken ?? string.Empty);
            }
            return result;
        }

        public string RequireAccessToken()
        {
            if (string.IsNullOrWhiteSpace(_tokenStore.AccessToken))
            {
                throw new InvalidOperationException("Access token is not available. Login first.");
            }
            return _tokenStore.AccessToken;
        }

        public void ClearSession() => _tokenStore.Clear();
    }
}
