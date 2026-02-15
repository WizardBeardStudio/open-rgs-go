using System;
using System.Threading;
using System.Threading.Tasks;
using Grpc.Core;
using Rgs.V1;
using WizardBeardStudio.Rgs.Models;

namespace WizardBeardStudio.Rgs.Services
{
    public sealed class SessionsClient
    {
        private readonly SessionsService.SessionsServiceClient _client;
        private readonly Func<string?> _accessTokenProvider;
        private readonly string _playerId;
        private readonly string _deviceId;
        private readonly string _userAgent;
        private readonly string _geo;

        public SessionsClient(
            SessionsService.SessionsServiceClient client,
            Func<string?> accessTokenProvider,
            string playerId,
            string deviceId,
            string userAgent,
            string geo)
        {
            _client = client;
            _accessTokenProvider = accessTokenProvider;
            _playerId = playerId;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
        }

        public async Task<SessionResult> StartSessionAsync(string deviceId, CancellationToken cancellationToken)
        {
            var request = new StartSessionRequest
            {
                Meta = BuildMeta(),
                PlayerId = _playerId,
                DeviceId = string.IsNullOrWhiteSpace(deviceId) ? _deviceId : deviceId,
                SessionTimeoutSeconds = 3600,
            };

            try
            {
                var response = await _client.StartSessionAsync(request, BuildHeaders(), cancellationToken: cancellationToken);
                if (response?.Meta == null)
                {
                    return new SessionResult { Success = false, ResultCode = "MISSING_META", DenialReason = "missing response metadata" };
                }

                return new SessionResult
                {
                    Success = (int)response.Meta.ResultCode == ProtoResultCode.Ok,
                    ResultCode = response.Meta.ResultCode.ToString(),
                    DenialReason = response.Meta.DenialReason,
                    RequestId = response.Meta.RequestId,
                    ServerTime = response.Meta.ServerTime,
                    SessionId = response.Session?.SessionId ?? string.Empty,
                    SessionState = response.Session?.State.ToString() ?? string.Empty,
                };
            }
            catch (RpcException ex)
            {
                return new SessionResult { Success = false, ResultCode = ex.StatusCode.ToString(), DenialReason = ex.Status.Detail };
            }
        }

        public async Task<OperationResult> EndSessionAsync(string sessionId, CancellationToken cancellationToken)
        {
            var request = new EndSessionRequest
            {
                Meta = BuildMeta(),
                SessionId = sessionId,
                Reason = "client_end",
            };

            try
            {
                var response = await _client.EndSessionAsync(request, BuildHeaders(), cancellationToken: cancellationToken);
                if (response?.Meta == null)
                {
                    return new OperationResult { Success = false, ResultCode = "MISSING_META", DenialReason = "missing response metadata" };
                }

                return new OperationResult
                {
                    Success = (int)response.Meta.ResultCode == ProtoResultCode.Ok,
                    ResultCode = response.Meta.ResultCode.ToString(),
                    DenialReason = response.Meta.DenialReason,
                    RequestId = response.Meta.RequestId,
                    ServerTime = response.Meta.ServerTime,
                };
            }
            catch (RpcException ex)
            {
                return new OperationResult { Success = false, ResultCode = ex.StatusCode.ToString(), DenialReason = ex.Status.Detail };
            }
        }

        private Metadata BuildHeaders()
        {
            var headers = new Metadata();
            var token = _accessTokenProvider();
            if (!string.IsNullOrWhiteSpace(token))
            {
                headers.Add("authorization", $"Bearer {token}");
            }
            return headers;
        }

        private RequestMeta BuildMeta()
        {
            return new RequestMeta
            {
                RequestId = Guid.NewGuid().ToString(),
                IdempotencyKey = string.Empty,
                Actor = new Actor { ActorId = _playerId, ActorType = (ActorType)1 },
                Source = new Source { DeviceId = _deviceId, UserAgent = _userAgent, Geo = _geo, Ip = string.Empty }
            };
        }
    }
}
