using System;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;
using Grpc.Core;
using Rgs.V1;
using WizardBeardStudio.Rgs.Core;
using WizardBeardStudio.Rgs.Models;

namespace WizardBeardStudio.Rgs.Services
{
    public sealed class IdentityClient
    {
        private readonly IdentityService.IdentityServiceClient? _grpcClient;
        private readonly IRgsTransport? _restTransport;
        private readonly string _deviceId;
        private readonly string _userAgent;
        private readonly string _geo;

        public IdentityClient(
            IdentityService.IdentityServiceClient client,
            string deviceId,
            string userAgent,
            string geo)
        {
            _grpcClient = client;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
        }

        public IdentityClient(
            IRgsTransport transport,
            string deviceId,
            string userAgent,
            string geo)
        {
            _restTransport = transport;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
        }

        public async Task<LoginResult> LoginPlayerAsync(string playerId, string pin, CancellationToken cancellationToken)
        {
            if (_restTransport != null)
            {
                return await LoginPlayerRestAsync(playerId, pin, cancellationToken);
            }
            return await LoginPlayerGrpcAsync(playerId, pin, cancellationToken);
        }

        private async Task<LoginResult> LoginPlayerGrpcAsync(string playerId, string pin, CancellationToken cancellationToken)
        {
            if (_grpcClient == null)
            {
                return new LoginResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "gRPC client not configured" };
            }

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
                var response = await _grpcClient.LoginAsync(request, cancellationToken: cancellationToken);
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

        private async Task<LoginResult> LoginPlayerRestAsync(string playerId, string pin, CancellationToken cancellationToken)
        {
            if (_restTransport == null)
            {
                return new LoginResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "REST transport not configured" };
            }

            var payload = new
            {
                meta = new
                {
                    requestId = Guid.NewGuid().ToString(),
                    idempotencyKey = string.Empty,
                    actor = new { actorId = playerId, actorType = "ACTOR_TYPE_PLAYER" },
                    source = new { ip = string.Empty, deviceId = _deviceId, userAgent = _userAgent, geo = _geo }
                },
                player = new { playerId, pin }
            };

            var json = JsonSerializer.Serialize(payload);
            var body = await _restTransport.PostJsonAsync("/v1/identity/login", json, null, cancellationToken);

            using var doc = JsonDocument.Parse(body);
            var meta = RestJson.ParseMeta(doc.RootElement);

            string accessToken = string.Empty;
            string refreshToken = string.Empty;
            string actorId = playerId;
            if (doc.RootElement.TryGetProperty("token", out var token))
            {
                accessToken = RestJson.GetString(token, "accessToken");
                refreshToken = RestJson.GetString(token, "refreshToken");
                if (token.TryGetProperty("actor", out var actor))
                {
                    actorId = RestJson.GetString(actor, "actorId");
                }
            }

            return new LoginResult
            {
                Success = meta.Success,
                ResultCode = meta.ResultCode,
                DenialReason = meta.DenialReason,
                RequestId = meta.RequestId,
                ServerTime = meta.ServerTime,
                AccessToken = accessToken,
                RefreshToken = refreshToken,
                ActorId = actorId,
            };
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
