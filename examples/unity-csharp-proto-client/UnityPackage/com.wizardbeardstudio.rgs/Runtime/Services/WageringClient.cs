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
    public sealed class WageringClient
    {
        private readonly WageringService.WageringServiceClient? _grpcClient;
        private readonly IRgsTransport? _restTransport;
        private readonly Func<string?> _accessTokenProvider;
        private readonly string _playerId;
        private readonly string _deviceId;
        private readonly string _userAgent;
        private readonly string _geo;

        public WageringClient(
            WageringService.WageringServiceClient client,
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

        public WageringClient(
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

        public async Task<WagerResult> PlaceWagerAsync(string gameId, long amountMinor, string currency, string idempotencyKey, CancellationToken cancellationToken)
        {
            if (_restTransport != null)
            {
                return await PlaceWagerRestAsync(gameId, amountMinor, currency, idempotencyKey, cancellationToken);
            }
            return await PlaceWagerGrpcAsync(gameId, amountMinor, currency, idempotencyKey, cancellationToken);
        }

        public async Task<OutcomeResult> SettleWagerAsync(string wagerId, long payoutMinor, string currency, CancellationToken cancellationToken)
        {
            if (_restTransport != null)
            {
                return await SettleWagerRestAsync(wagerId, payoutMinor, currency, cancellationToken);
            }
            return await SettleWagerGrpcAsync(wagerId, payoutMinor, currency, cancellationToken);
        }

        private async Task<WagerResult> PlaceWagerGrpcAsync(string gameId, long amountMinor, string currency, string idempotencyKey, CancellationToken cancellationToken)
        {
            if (_grpcClient == null)
            {
                return new WagerResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "gRPC client not configured" };
            }

            var request = new PlaceWagerRequest
            {
                Meta = BuildMeta(idempotencyKey),
                PlayerId = _playerId,
                GameId = gameId,
                Stake = new Money { AmountMinor = amountMinor, Currency = currency }
            };

            try
            {
                var response = await _grpcClient.PlaceWagerAsync(request, BuildHeaders(), cancellationToken: cancellationToken);
                if (response?.Meta == null)
                {
                    return new WagerResult { Success = false, ResultCode = "MISSING_META", DenialReason = "missing response metadata" };
                }

                return new WagerResult
                {
                    Success = (int)response.Meta.ResultCode == ProtoResultCode.Ok,
                    ResultCode = response.Meta.ResultCode.ToString(),
                    DenialReason = response.Meta.DenialReason,
                    RequestId = response.Meta.RequestId,
                    ServerTime = response.Meta.ServerTime,
                    WagerId = response.Wager?.WagerId ?? string.Empty,
                    WagerStatus = response.Wager?.Status.ToString() ?? string.Empty,
                };
            }
            catch (RpcException ex)
            {
                return new WagerResult { Success = false, ResultCode = ex.StatusCode.ToString(), DenialReason = ex.Status.Detail };
            }
        }

        private async Task<OutcomeResult> SettleWagerGrpcAsync(string wagerId, long payoutMinor, string currency, CancellationToken cancellationToken)
        {
            if (_grpcClient == null)
            {
                return new OutcomeResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "gRPC client not configured" };
            }

            var request = new SettleWagerRequest
            {
                Meta = BuildMeta(string.Empty),
                WagerId = wagerId,
                Payout = new Money { AmountMinor = payoutMinor, Currency = currency },
                OutcomeRef = "unity-sample-outcome"
            };

            try
            {
                var response = await _grpcClient.SettleWagerAsync(request, BuildHeaders(), cancellationToken: cancellationToken);
                if (response?.Meta == null)
                {
                    return new OutcomeResult { Success = false, ResultCode = "MISSING_META", DenialReason = "missing response metadata" };
                }

                return new OutcomeResult
                {
                    Success = (int)response.Meta.ResultCode == ProtoResultCode.Ok,
                    ResultCode = response.Meta.ResultCode.ToString(),
                    DenialReason = response.Meta.DenialReason,
                    RequestId = response.Meta.RequestId,
                    ServerTime = response.Meta.ServerTime,
                    WagerId = response.Wager?.WagerId ?? string.Empty,
                    WagerStatus = response.Wager?.Status.ToString() ?? string.Empty,
                };
            }
            catch (RpcException ex)
            {
                return new OutcomeResult { Success = false, ResultCode = ex.StatusCode.ToString(), DenialReason = ex.Status.Detail };
            }
        }

        private async Task<WagerResult> PlaceWagerRestAsync(string gameId, long amountMinor, string currency, string idempotencyKey, CancellationToken cancellationToken)
        {
            if (_restTransport == null)
            {
                return new WagerResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "REST transport not configured" };
            }

            var payload = new
            {
                meta = BuildMetaJson(idempotencyKey),
                playerId = _playerId,
                gameId,
                stake = new { amountMinor, currency }
            };

            var body = await _restTransport.PostJsonAsync("/v1/wagering/wagers", JsonSerializer.Serialize(payload), BuildHeaderMap(), cancellationToken);
            using var doc = JsonDocument.Parse(body);
            var meta = RestJson.ParseMeta(doc.RootElement);

            string wagerId = string.Empty;
            string wagerStatus = string.Empty;
            if (doc.RootElement.TryGetProperty("wager", out var wager))
            {
                wagerId = RestJson.GetString(wager, "wagerId");
                wagerStatus = RestJson.GetString(wager, "status");
            }

            return new WagerResult
            {
                Success = meta.Success,
                ResultCode = meta.ResultCode,
                DenialReason = meta.DenialReason,
                RequestId = meta.RequestId,
                ServerTime = meta.ServerTime,
                WagerId = wagerId,
                WagerStatus = wagerStatus,
            };
        }

        private async Task<OutcomeResult> SettleWagerRestAsync(string wagerId, long payoutMinor, string currency, CancellationToken cancellationToken)
        {
            if (_restTransport == null)
            {
                return new OutcomeResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "REST transport not configured" };
            }

            var payload = new
            {
                meta = BuildMetaJson(string.Empty),
                wagerId,
                payout = new { amountMinor = payoutMinor, currency },
                outcomeRef = "unity-sample-outcome"
            };

            var path = "/v1/wagering/wagers/" + Uri.EscapeDataString(wagerId) + ":settle";
            var body = await _restTransport.PostJsonAsync(path, JsonSerializer.Serialize(payload), BuildHeaderMap(), cancellationToken);
            using var doc = JsonDocument.Parse(body);
            var meta = RestJson.ParseMeta(doc.RootElement);

            string resolvedWagerId = string.Empty;
            string wagerStatus = string.Empty;
            if (doc.RootElement.TryGetProperty("wager", out var wager))
            {
                resolvedWagerId = RestJson.GetString(wager, "wagerId");
                wagerStatus = RestJson.GetString(wager, "status");
            }

            return new OutcomeResult
            {
                Success = meta.Success,
                ResultCode = meta.ResultCode,
                DenialReason = meta.DenialReason,
                RequestId = meta.RequestId,
                ServerTime = meta.ServerTime,
                WagerId = resolvedWagerId,
                WagerStatus = wagerStatus,
            };
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

        private RequestMeta BuildMeta(string idempotencyKey)
        {
            return new RequestMeta
            {
                RequestId = Guid.NewGuid().ToString(),
                IdempotencyKey = idempotencyKey,
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
