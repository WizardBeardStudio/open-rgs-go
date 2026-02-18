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
    internal interface IReportingRpcClient
    {
        Task<GenerateReportResponse> GenerateReportAsync(GenerateReportRequest request, Metadata headers, CancellationToken cancellationToken);
    }

    internal sealed class ReportingRpcClientAdapter : IReportingRpcClient
    {
        private readonly ReportingService.ReportingServiceClient _client;

        public ReportingRpcClientAdapter(ReportingService.ReportingServiceClient client)
        {
            _client = client;
        }

        public Task<GenerateReportResponse> GenerateReportAsync(GenerateReportRequest request, Metadata headers, CancellationToken cancellationToken)
            => _client.GenerateReportAsync(request, headers, cancellationToken: cancellationToken).ResponseAsync;
    }

    public sealed class ReportingClient
    {
        private readonly IReportingRpcClient? _grpcClient;
        private readonly IRgsTransport? _restTransport;
        private readonly Func<string?> _accessTokenProvider;
        private readonly string _actorId;
        private readonly string _deviceId;
        private readonly string _userAgent;
        private readonly string _geo;
        private readonly string _operatorId;

        public ReportingClient(
            ReportingService.ReportingServiceClient client,
            Func<string?> accessTokenProvider,
            string actorId,
            string deviceId,
            string userAgent,
            string geo,
            string operatorId)
        {
            _grpcClient = new ReportingRpcClientAdapter(client);
            _accessTokenProvider = accessTokenProvider;
            _actorId = actorId;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
            _operatorId = operatorId;
        }

        public ReportingClient(
            IRgsTransport transport,
            Func<string?> accessTokenProvider,
            string actorId,
            string deviceId,
            string userAgent,
            string geo,
            string operatorId)
        {
            _restTransport = transport;
            _accessTokenProvider = accessTokenProvider;
            _actorId = actorId;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
            _operatorId = operatorId;
        }

        internal ReportingClient(
            IReportingRpcClient client,
            Func<string?> accessTokenProvider,
            string actorId,
            string deviceId,
            string userAgent,
            string geo,
            string operatorId)
        {
            _grpcClient = client;
            _accessTokenProvider = accessTokenProvider;
            _actorId = actorId;
            _deviceId = deviceId;
            _userAgent = userAgent;
            _geo = geo;
            _operatorId = operatorId;
        }

        public async Task<OperationResult> GenerateReportAsync(string reportType, string interval, CancellationToken cancellationToken)
        {
            if (_restTransport != null)
            {
                return await GenerateReportRestAsync(reportType, interval, cancellationToken);
            }
            return await GenerateReportGrpcAsync(reportType, interval, cancellationToken);
        }

        private async Task<OperationResult> GenerateReportGrpcAsync(string reportType, string interval, CancellationToken cancellationToken)
        {
            if (_grpcClient == null)
            {
                return new OperationResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "gRPC client not configured" };
            }

            var request = new GenerateReportRequest
            {
                Meta = BuildMeta(),
                ReportType = (ReportType)MapReportType(reportType),
                Interval = (ReportInterval)MapReportInterval(interval),
                Format = (ReportFormat)1,
                OperatorId = _operatorId,
            };

            try
            {
                var response = await _grpcClient.GenerateReportAsync(request, BuildHeaders(), cancellationToken);
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

        private async Task<OperationResult> GenerateReportRestAsync(string reportType, string interval, CancellationToken cancellationToken)
        {
            if (_restTransport == null)
            {
                return new OperationResult { Success = false, ResultCode = "TRANSPORT_NOT_CONFIGURED", DenialReason = "REST transport not configured" };
            }

            var payload = new
            {
                meta = BuildMetaJson(string.Empty),
                reportType = MapReportTypeName(reportType),
                interval = MapReportIntervalName(interval),
                format = "REPORT_FORMAT_JSON",
                operatorId = _operatorId,
            };

            var body = await _restTransport.PostJsonAsync("/v1/reporting/runs", JsonSerializer.Serialize(payload), BuildHeaderMap(), cancellationToken);
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

        private static int MapReportType(string reportType)
        {
            var normalized = (reportType ?? string.Empty).Trim().ToUpperInvariant();
            return normalized switch
            {
                "SIGNIFICANT_EVENTS_ALTERATIONS" => 1,
                "CASHLESS_LIABILITY_SUMMARY" => 2,
                "ACCOUNT_TRANSACTION_STATEMENT" => 3,
                _ => 1,
            };
        }

        private static string MapReportTypeName(string reportType)
        {
            var normalized = (reportType ?? string.Empty).Trim().ToUpperInvariant();
            return normalized switch
            {
                "SIGNIFICANT_EVENTS_ALTERATIONS" => "REPORT_TYPE_SIGNIFICANT_EVENTS_ALTERATIONS",
                "CASHLESS_LIABILITY_SUMMARY" => "REPORT_TYPE_CASHLESS_LIABILITY_SUMMARY",
                "ACCOUNT_TRANSACTION_STATEMENT" => "REPORT_TYPE_ACCOUNT_TRANSACTION_STATEMENT",
                _ => "REPORT_TYPE_SIGNIFICANT_EVENTS_ALTERATIONS",
            };
        }

        private static int MapReportInterval(string interval)
        {
            var normalized = (interval ?? string.Empty).Trim().ToUpperInvariant();
            return normalized switch
            {
                "DTD" => 1,
                "MTD" => 2,
                "YTD" => 3,
                "LTD" => 4,
                _ => 1,
            };
        }

        private static string MapReportIntervalName(string interval)
        {
            var normalized = (interval ?? string.Empty).Trim().ToUpperInvariant();
            return normalized switch
            {
                "DTD" => "REPORT_INTERVAL_DTD",
                "MTD" => "REPORT_INTERVAL_MTD",
                "YTD" => "REPORT_INTERVAL_YTD",
                "LTD" => "REPORT_INTERVAL_LTD",
                _ => "REPORT_INTERVAL_DTD",
            };
        }
    }
}
