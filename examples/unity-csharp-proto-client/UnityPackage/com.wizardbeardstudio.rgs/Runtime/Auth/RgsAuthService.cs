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
                _tokenStore.Save(result.AccessToken ?? string.Empty, result.RefreshToken ?? string.Empty, result.ActorId ?? playerId);
            }
            return result;
        }

        public async Task<LoginResult> RefreshTokenAsync(CancellationToken cancellationToken)
        {
            if (string.IsNullOrWhiteSpace(_tokenStore.RefreshToken))
            {
                return new LoginResult
                {
                    Success = false,
                    ResultCode = "NO_REFRESH_TOKEN",
                    DenialReason = "refresh token is not available",
                };
            }

            var actorId = _tokenStore.ActorId ?? string.Empty;
            if (string.IsNullOrWhiteSpace(actorId))
            {
                return new LoginResult
                {
                    Success = false,
                    ResultCode = "NO_ACTOR_ID",
                    DenialReason = "actor id is not available for refresh request",
                };
            }

            var result = await _identityClient.RefreshPlayerTokenAsync(actorId, _tokenStore.RefreshToken, cancellationToken);
            if (result.Success)
            {
                _tokenStore.Save(result.AccessToken ?? string.Empty, result.RefreshToken ?? string.Empty, result.ActorId ?? actorId);
            }
            return result;
        }

        public async Task<OperationResult> LogoutAsync(CancellationToken cancellationToken)
        {
            var refreshToken = _tokenStore.RefreshToken ?? string.Empty;
            var actorId = _tokenStore.ActorId ?? string.Empty;
            if (string.IsNullOrWhiteSpace(refreshToken) || string.IsNullOrWhiteSpace(actorId))
            {
                _tokenStore.Clear();
                return new OperationResult
                {
                    Success = true,
                    ResultCode = "NOOP",
                    DenialReason = string.Empty,
                };
            }

            var result = await _identityClient.LogoutAsync(actorId, refreshToken, cancellationToken);
            _tokenStore.Clear();
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
