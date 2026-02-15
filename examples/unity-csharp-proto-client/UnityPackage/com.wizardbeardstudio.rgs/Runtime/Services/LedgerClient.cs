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
    public sealed class LedgerClient
    {
        private readonly LedgerService.LedgerServiceClient? _grpcClient;
        private readonly IRgsTransport? _restTransport;
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
            _grpcClient = client;
            _accessTokenProvider = accessTokenProvider;
            _playerId = playerId;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
        }

        public LedgerClient(
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

        public async Task<BalanceResult> GetBalanceAsync(string accountId, CancellationToken cancellationToken)
        {
            if (_restTransport != null)
            {
                return await GetBalanceRestAsync(accountId, cancellationToken);
            }
            return await GetBalanceGrpcAsync(accountId, cancellationToken);
        }

        private async Task<BalanceResult> GetBalanceGrpcAsync(string accountId, CancellationToken cancellationToken)
        {
            if (_grpcClient == null)
            {
                return new BalanceResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "gRPC client not configured" };
            }

            var request = new GetBalanceRequest
            {
                Meta = BuildMeta(),
                AccountId = accountId,
            };

            var headers = BuildHeaders();

            try
            {
                var response = await _grpcClient.GetBalanceAsync(request, headers, cancellationToken: cancellationToken);
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

        private async Task<BalanceResult> GetBalanceRestAsync(string accountId, CancellationToken cancellationToken)
        {
            if (_restTransport == null)
            {
                return new BalanceResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "REST transport not configured" };
            }

            var path = "/v1/ledger/accounts/" + Uri.EscapeDataString(accountId) + "/balance";
            var body = await _restTransport.GetJsonAsync(path, BuildHeaderMap(), cancellationToken);
            using var doc = JsonDocument.Parse(body);
            var meta = RestJson.ParseMeta(doc.RootElement);

            long availableMinor = 0;
            long pendingMinor = 0;
            string currency = string.Empty;

            if (doc.RootElement.TryGetProperty("availableBalance", out var available))
            {
                availableMinor = RestJson.GetInt64(available, "amountMinor");
                currency = RestJson.GetString(available, "currency");
            }
            if (doc.RootElement.TryGetProperty("pendingBalance", out var pending))
            {
                pendingMinor = RestJson.GetInt64(pending, "amountMinor");
            }

            return new BalanceResult
            {
                Success = meta.Success,
                ResultCode = meta.ResultCode,
                DenialReason = meta.DenialReason,
                RequestId = meta.RequestId,
                ServerTime = meta.ServerTime,
                AvailableMinor = availableMinor,
                PendingMinor = pendingMinor,
                Currency = currency,
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
