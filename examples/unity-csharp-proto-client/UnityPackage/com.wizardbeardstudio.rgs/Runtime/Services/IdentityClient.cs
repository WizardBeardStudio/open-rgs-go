using System;
using System.Threading;
using System.Threading.Tasks;
using Grpc.Core;
using Rgs.V1;
using WizardBeardStudio.Rgs.Models;

namespace WizardBeardStudio.Rgs.Services
{
    public sealed class IdentityClient
    {
        private readonly IdentityService.IdentityServiceClient _client;
        private readonly string _deviceId;
        private readonly string _userAgent;
        private readonly string _geo;

        public IdentityClient(
            IdentityService.IdentityServiceClient client,
            string deviceId,
            string userAgent,
            string geo)
        {
            _client = client;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
        }

        public async Task<LoginResult> LoginPlayerAsync(string playerId, string pin, CancellationToken cancellationToken)
        {
            var request = new LoginRequest
            {
                Meta = BuildMeta(playerId),
                Player = new PlayerCredentials
                {
                    PlayerId = playerId,
                    Pin = pin,
                }
            };

            try
            {
                var response = await _client.LoginAsync(request, cancellationToken: cancellationToken);
                if (response?.Meta == null)
                {
                    return new LoginResult
                    {
                        Success = false,
                        ResultCode = "MISSING_META",
                        DenialReason = "missing response metadata",
                    };
                }

                var code = (int)response.Meta.ResultCode;
                var success = code == ProtoResultCode.Ok;

                return new LoginResult
                {
                    Success = success,
                    ResultCode = response.Meta.ResultCode.ToString(),
                    DenialReason = response.Meta.DenialReason,
                    RequestId = response.Meta.RequestId,
                    ServerTime = response.Meta.ServerTime,
                    AccessToken = response.Token?.AccessToken,
                    RefreshToken = response.Token?.RefreshToken,
                    ActorId = response.Token?.Actor?.ActorId ?? playerId,
                };
            }
            catch (RpcException ex)
            {
                return new LoginResult
                {
                    Success = false,
                    ResultCode = ex.StatusCode.ToString(),
                    DenialReason = ex.Status.Detail,
                };
            }
        }

        private RequestMeta BuildMeta(string playerId)
        {
            return new RequestMeta
            {
                RequestId = Guid.NewGuid().ToString(),
                IdempotencyKey = string.Empty,
                Actor = new Actor
                {
                    ActorId = playerId,
                    ActorType = (ActorType)1,
                },
                Source = new Source
                {
                    DeviceId = _deviceId,
                    UserAgent = _userAgent,
                    Geo = _geo,
                    Ip = string.Empty,
                }
            };
        }
    }
}
