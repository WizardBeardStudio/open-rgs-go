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
    internal interface IEventsRpcClient
    {
        Task<SubmitSignificantEventResponse> SubmitSignificantEventAsync(SubmitSignificantEventRequest request, Metadata headers, CancellationToken cancellationToken);
    }

    internal sealed class EventsRpcClientAdapter : IEventsRpcClient
    {
        private readonly EventsService.EventsServiceClient _client;

        public EventsRpcClientAdapter(EventsService.EventsServiceClient client)
        {
            _client = client;
        }

        public Task<SubmitSignificantEventResponse> SubmitSignificantEventAsync(SubmitSignificantEventRequest request, Metadata headers, CancellationToken cancellationToken)
            => _client.SubmitSignificantEventAsync(request, headers, cancellationToken: cancellationToken).ResponseAsync;
    }

    public sealed class EventsClient
    {
        private readonly IEventsRpcClient? _grpcClient;
        private readonly IRgsTransport? _restTransport;
        private readonly Func<string?> _accessTokenProvider;
        private readonly string _actorId;
        private readonly string _deviceId;
        private readonly string _userAgent;
        private readonly string _geo;
        private readonly string _equipmentId;

        public EventsClient(
            EventsService.EventsServiceClient client,
            Func<string?> accessTokenProvider,
            string actorId,
            string deviceId,
            string userAgent,
            string geo,
            string equipmentId)
        {
            _grpcClient = new EventsRpcClientAdapter(client);
            _accessTokenProvider = accessTokenProvider;
            _actorId = actorId;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
            _equipmentId = equipmentId;
        }

        public EventsClient(
            IRgsTransport transport,
            Func<string?> accessTokenProvider,
            string actorId,
            string deviceId,
            string userAgent,
            string geo,
            string equipmentId)
        {
            _restTransport = transport;
            _accessTokenProvider = accessTokenProvider;
            _actorId = actorId;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
            _equipmentId = equipmentId;
        }

        internal EventsClient(
            IEventsRpcClient client,
            Func<string?> accessTokenProvider,
            string actorId,
            string deviceId,
            string userAgent,
            string geo,
            string equipmentId)
        {
            _grpcClient = client;
            _accessTokenProvider = accessTokenProvider;
            _actorId = actorId;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
            _equipmentId = equipmentId;
        }

        public async Task<OperationResult> SubmitSignificantEventAsync(string eventCode, string description, CancellationToken cancellationToken)
        {
            if (_restTransport != null)
            {
                return await SubmitSignificantEventRestAsync(eventCode, description, cancellationToken);
            }
            return await SubmitSignificantEventGrpcAsync(eventCode, description, cancellationToken);
        }

        private async Task<OperationResult> SubmitSignificantEventGrpcAsync(string eventCode, string description, CancellationToken cancellationToken)
        {
            if (_grpcClient == null)
            {
                return new OperationResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "gRPC client not configured" };
            }

            var now = DateTimeOffset.UtcNow.ToString("O");
            var request = new SubmitSignificantEventRequest
            {
                Meta = BuildMeta(),
                Event = new SignificantEvent
                {
                    EventId = Guid.NewGuid().ToString(),
                    EquipmentId = _equipmentId,
                    EventCode = eventCode,
                    LocalizedDescription = description,
                    Severity = (EventSeverity)1,
                    OccurredAt = now,
                }
            };

            try
            {
                var response = await _grpcClient.SubmitSignificantEventAsync(request, BuildHeaders(), cancellationToken);
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

        private async Task<OperationResult> SubmitSignificantEventRestAsync(string eventCode, string description, CancellationToken cancellationToken)
        {
            if (_restTransport == null)
            {
                return new OperationResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "REST transport not configured" };
            }

            var now = DateTimeOffset.UtcNow.ToString("O");
            var payload = new
            {
                meta = BuildMetaJson(string.Empty),
                @event = new
                {
                    eventId = Guid.NewGuid().ToString(),
                    equipmentId = _equipmentId,
                    eventCode,
                    localizedDescription = description,
                    severity = "EVENT_SEVERITY_INFO",
                    occurredAt = now
                }
            };

            var body = await _restTransport.PostJsonAsync("/v1/events/significant", JsonSerializer.Serialize(payload), BuildHeaderMap(), cancellationToken);
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
                Actor = new Actor { ActorId = _actorId, ActorType = (ActorType)1 },
                Source = new Source { DeviceId = _deviceId, UserAgent = _userAgent, Geo = _geo, Ip = string.Empty }
            };
        }

        private object BuildMetaJson(string idempotencyKey)
        {
            return new
            {
                requestId = Guid.NewGuid().ToString(),
                idempotencyKey,
                actor = new { actorId = _actorId, actorType = "ACTOR_TYPE_PLAYER" },
                source = new { ip = string.Empty, deviceId = _deviceId, userAgent = _userAgent, geo = _geo }
            };
        }
    }
}
