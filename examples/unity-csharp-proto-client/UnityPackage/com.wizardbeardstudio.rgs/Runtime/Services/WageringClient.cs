using System;
using System.Threading;
using System.Threading.Tasks;
using Grpc.Core;
using Rgs.V1;
using WizardBeardStudio.Rgs.Models;

namespace WizardBeardStudio.Rgs.Services
{
    public sealed class WageringClient
    {
        private readonly WageringService.WageringServiceClient _client;
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
            _client = client;
            _accessTokenProvider = accessTokenProvider;
            _playerId = playerId;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
        }

        public async Task<WagerResult> PlaceWagerAsync(string gameId, long amountMinor, string currency, string idempotencyKey, CancellationToken cancellationToken)
        {
            var request = new PlaceWagerRequest
            {
                Meta = BuildMeta(idempotencyKey),
                PlayerId = _playerId,
                GameId = gameId,
                Stake = new Money { AmountMinor = amountMinor, Currency = currency }
            };

            try
            {
                var response = await _client.PlaceWagerAsync(request, BuildHeaders(), cancellationToken: cancellationToken);
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

        public async Task<OutcomeResult> SettleWagerAsync(string wagerId, long payoutMinor, string currency, CancellationToken cancellationToken)
        {
            var request = new SettleWagerRequest
            {
                Meta = BuildMeta(string.Empty),
                WagerId = wagerId,
                Payout = new Money { AmountMinor = payoutMinor, Currency = currency },
                OutcomeRef = "unity-sample-outcome"
            };

            try
            {
                var response = await _client.SettleWagerAsync(request, BuildHeaders(), cancellationToken: cancellationToken);
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
    }
}
