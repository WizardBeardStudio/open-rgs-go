using System;
using System.Collections.Generic;
using System.Text.Json;
using System.Threading;
using System.Threading.Tasks;
using Grpc.Core;
using Rgs.V1;
using WizardBeardStudio.Rgs.Core;
using WizardBeardStudio.Rgs.Models;

namespace WizardBeardStudio.Rgs.Services
{
    internal interface ISessionsRpcClient
    {
        Task<StartSessionResponse> StartSessionAsync(StartSessionRequest request, Metadata headers, CancellationToken cancellationToken);
        Task<EndSessionResponse> EndSessionAsync(EndSessionRequest request, Metadata headers, CancellationToken cancellationToken);
    }

    internal sealed class SessionsRpcClientAdapter : ISessionsRpcClient
    {
        private readonly SessionsService.SessionsServiceClient _client;

        public SessionsRpcClientAdapter(SessionsService.SessionsServiceClient client)
        {
            _client = client;
        }

        public Task<StartSessionResponse> StartSessionAsync(StartSessionRequest request, Metadata headers, CancellationToken cancellationToken)
            => _client.StartSessionAsync(request, headers, cancellationToken: cancellationToken).ResponseAsync;

        public Task<EndSessionResponse> EndSessionAsync(EndSessionRequest request, Metadata headers, CancellationToken cancellationToken)
            => _client.EndSessionAsync(request, headers, cancellationToken: cancellationToken).ResponseAsync;
    }

    public sealed class SessionsClient
    {
        private readonly ISessionsRpcClient? _grpcClient;
        private readonly IRgsTransport? _restTransport;
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
            _grpcClient = new SessionsRpcClientAdapter(client);
            _accessTokenProvider = accessTokenProvider;
            _playerId = playerId;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
        }

        public SessionsClient(
            IRgsTransport transport,
            Func<string?> accessTokenProvider,
            string playerId,
            string deviceId,
            string userAgent,
            string geo)
        {
            _restTransport = transport;
            _accessTokenProvider = accessTokenProvider;
            _playerId = playerId;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
        }

        internal SessionsClient(
            ISessionsRpcClient client,
            Func<string?> accessTokenProvider,
            string playerId,
            string deviceId,
            string userAgent,
            string geo)
        {
            _grpcClient = client;
            _accessTokenProvider = accessTokenProvider;
            _playerId = playerId;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
        }

        public async Task<SessionResult> StartSessionAsync(string deviceId, CancellationToken cancellationToken)
        {
            if (_restTransport != null)
            {
                return await StartSessionRestAsync(deviceId, cancellationToken);
            }
            return await StartSessionGrpcAsync(deviceId, cancellationToken);
        }

        public async Task<OperationResult> EndSessionAsync(string sessionId, CancellationToken cancellationToken)
        {
            if (_restTransport != null)
            {
                return await EndSessionRestAsync(sessionId, cancellationToken);
            }
            return await EndSessionGrpcAsync(sessionId, cancellationToken);
        }

        private async Task<SessionResult> StartSessionGrpcAsync(string deviceId, CancellationToken cancellationToken)
        {
            if (_grpcClient == null)
            {
                return new SessionResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "gRPC client not configured" };
            }

            var request = new StartSessionRequest
            {
                Meta = BuildMeta(),
                PlayerId = _playerId,
                DeviceId = string.IsNullOrWhiteSpace(deviceId) ? _deviceId : deviceId,
                SessionTimeoutSeconds = 3600,
            };

            try
            {
                var response = await _grpcClient.StartSessionAsync(request, BuildHeaders(), cancellationToken: cancellationToken);
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

        private async Task<OperationResult> EndSessionGrpcAsync(string sessionId, CancellationToken cancellationToken)
        {
            if (_grpcClient == null)
            {
                return new OperationResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "gRPC client not configured" };
            }

            var request = new EndSessionRequest
            {
                Meta = BuildMeta(),
                SessionId = sessionId,
                Reason = "client_end",
            };

            try
            {
                var response = await _grpcClient.EndSessionAsync(request, BuildHeaders(), cancellationToken: cancellationToken);
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

        private async Task<SessionResult> StartSessionRestAsync(string deviceId, CancellationToken cancellationToken)
        {
            if (_restTransport == null)
            {
                return new SessionResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "REST transport not configured" };
            }

            var payload = new
            {
                meta = BuildMetaJson(string.Empty),
                playerId = _playerId,
                deviceId = string.IsNullOrWhiteSpace(deviceId) ? _deviceId : deviceId,
                sessionTimeoutSeconds = 3600,
            };

            var body = await _restTransport.PostJsonAsync("/v1/sessions:start", JsonSerializer.Serialize(payload), BuildHeaderMap(), cancellationToken);
            using var doc = JsonDocument.Parse(body);
            var meta = RestJson.ParseMeta(doc.RootElement);

            string sessionId = string.Empty;
            string sessionState = string.Empty;
            if (doc.RootElement.TryGetProperty("session", out var session))
            {
                sessionId = RestJson.GetString(session, "sessionId");
                sessionState = RestJson.GetString(session, "state");
            }

            return new SessionResult
            {
                Success = meta.Success,
                ResultCode = meta.ResultCode,
                DenialReason = meta.DenialReason,
                RequestId = meta.RequestId,
                ServerTime = meta.ServerTime,
                SessionId = sessionId,
                SessionState = sessionState,
            };
        }

        private async Task<OperationResult> EndSessionRestAsync(string sessionId, CancellationToken cancellationToken)
        {
            if (_restTransport == null)
            {
                return new OperationResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "REST transport not configured" };
            }

            var payload = new
            {
                meta = BuildMetaJson(string.Empty),
                sessionId,
                reason = "client_end"
            };

            var body = await _restTransport.PostJsonAsync("/v1/sessions:end", JsonSerializer.Serialize(payload), BuildHeaderMap(), cancellationToken);
            using var doc = JsonDocument.Parse(body);
            return RestJson.ToOperationResult(RestJson.ParseMeta(doc.RootElement));
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

        private IDictionary<string, string> BuildHeaderMap()
        {
            var headers = new Dictionary<string, string>();
            var token = _accessTokenProvider();
            if (!string.IsNullOrWhiteSpace(token))
            {
                headers["authorization"] = "Bearer " + token;
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

        private object BuildMetaJson(string idempotencyKey)
        {
            return new
            {
                requestId = Guid.NewGuid().ToString(),
                idempotencyKey,
                actor = new { actorId = _playerId, actorType = "ACTOR_TYPE_PLAYER" },
                source = new { ip = string.Empty, deviceId = _deviceId, userAgent = _userAgent, geo = _geo }
            };
        }
    }
}
