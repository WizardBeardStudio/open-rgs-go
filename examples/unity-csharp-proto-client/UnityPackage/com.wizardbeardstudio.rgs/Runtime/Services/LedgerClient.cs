using System;
using System.Threading;
using System.Threading.Tasks;
using Grpc.Core;
using Rgs.V1;
using WizardBeardStudio.Rgs.Models;

namespace WizardBeardStudio.Rgs.Services
{
    public sealed class LedgerClient
    {
        private readonly LedgerService.LedgerServiceClient _client;
        private readonly Func<string?> _accessTokenProvider;
        private readonly string _playerId;
        private readonly string _deviceId;
        private readonly string _userAgent;
        private readonly string _geo;

        public LedgerClient(
            LedgerService.LedgerServiceClient client,
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

        public async Task<BalanceResult> GetBalanceAsync(string accountId, CancellationToken cancellationToken)
        {
            var request = new GetBalanceRequest
            {
                Meta = BuildMeta(),
                AccountId = accountId,
            };

            var headers = new Metadata();
            var token = _accessTokenProvider();
            if (!string.IsNullOrWhiteSpace(token))
            {
                headers.Add("authorization", $"Bearer {token}");
            }

            try
            {
                var response = await _client.GetBalanceAsync(request, headers, cancellationToken: cancellationToken);
                if (response?.Meta == null)
                {
                    return new BalanceResult
                    {
                        Success = false,
                        ResultCode = "MISSING_META",
                        DenialReason = "missing response metadata",
                    };
                }

                var code = (int)response.Meta.ResultCode;
                var success = code == ProtoResultCode.Ok;

                return new BalanceResult
                {
                    Success = success,
                    ResultCode = response.Meta.ResultCode.ToString(),
                    DenialReason = response.Meta.DenialReason,
                    RequestId = response.Meta.RequestId,
                    ServerTime = response.Meta.ServerTime,
                    AvailableMinor = response.AvailableBalance?.AmountMinor ?? 0,
                    PendingMinor = response.PendingBalance?.AmountMinor ?? 0,
                    Currency = response.AvailableBalance?.Currency ?? string.Empty,
                };
            }
            catch (RpcException ex)
            {
                return new BalanceResult
                {
                    Success = false,
                    ResultCode = ex.StatusCode.ToString(),
                    DenialReason = ex.Status.Detail,
                };
            }
        }

        private RequestMeta BuildMeta()
        {
            return new RequestMeta
            {
                RequestId = Guid.NewGuid().ToString(),
                IdempotencyKey = string.Empty,
                Actor = new Actor
                {
                    ActorId = _playerId,
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
