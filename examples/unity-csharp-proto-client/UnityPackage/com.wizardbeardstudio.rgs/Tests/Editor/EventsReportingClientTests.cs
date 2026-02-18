using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;
using Grpc.Core;
using NUnit.Framework;
using Rgs.V1;
using WizardBeardStudio.Rgs.Core;
using WizardBeardStudio.Rgs.Services;

namespace WizardBeardStudio.Rgs.Tests.Editor
{
    public sealed class EventsReportingClientTests
    {
        [Test]
        public async Task EventsSubmitSignificantEvent_GrpcAndRest_MapOperationResult()
        {
            var grpc = new EventsClient(new FakeEventsRpcClient(), () => "token", "player-1", "dev-1", "ua", string.Empty, "eq-1");
            var rest = new EventsClient(new FakeTransport(), () => "token", "player-1", "dev-1", "ua", string.Empty, "eq-1");

            var grpcResult = await grpc.SubmitSignificantEventAsync("E1001", "Door open", CancellationToken.None);
            var restResult = await rest.SubmitSignificantEventAsync("E1001", "Door open", CancellationToken.None);

            Assert.That(grpcResult.Success, Is.True);
            Assert.That(restResult.Success, Is.True);
            Assert.That(grpcResult.ResultCode, Is.Not.Empty);
            Assert.That(restResult.ResultCode, Is.Not.Empty);
        }

        [Test]
        public async Task ReportingGenerateReport_GrpcAndRest_MapOperationResult()
        {
            var grpc = new ReportingClient(new FakeReportingRpcClient(), () => "token", "player-1", "dev-1", "ua", string.Empty, "op-1");
            var rest = new ReportingClient(new FakeTransport(), () => "token", "player-1", "dev-1", "ua", string.Empty, "op-1");

            var grpcResult = await grpc.GenerateReportAsync("CASHLESS_LIABILITY_SUMMARY", "MTD", CancellationToken.None);
            var restResult = await rest.GenerateReportAsync("CASHLESS_LIABILITY_SUMMARY", "MTD", CancellationToken.None);

            Assert.That(grpcResult.Success, Is.True);
            Assert.That(restResult.Success, Is.True);
            Assert.That(grpcResult.ResultCode, Is.Not.Empty);
            Assert.That(restResult.ResultCode, Is.Not.Empty);
        }

        private sealed class FakeTransport : IRgsTransport
        {
            public Task<string> PostJsonAsync(string path, string jsonBody, IDictionary<string, string>? headers, CancellationToken cancellationToken)
            {
                if (path == "/v1/events/significant" || path == "/v1/reporting/runs")
                {
                    return Task.FromResult("{" +
                                           "\"meta\":{\"requestId\":\"req-1\",\"resultCode\":\"RESULT_CODE_OK\",\"denialReason\":\"\",\"serverTime\":\"2026-02-15T00:00:00Z\"}}");
                }

                return Task.FromResult("{" +
                                       "\"meta\":{\"requestId\":\"req-x\",\"resultCode\":\"RESULT_CODE_ERROR\",\"denialReason\":\"unexpected\",\"serverTime\":\"2026-02-15T00:00:00Z\"}}");
            }

            public Task<string> GetJsonAsync(string path, IDictionary<string, string>? headers, CancellationToken cancellationToken)
            {
                return Task.FromResult("{}");
            }
        }

        private sealed class FakeEventsRpcClient : IEventsRpcClient
        {
            public Task<SubmitSignificantEventResponse> SubmitSignificantEventAsync(SubmitSignificantEventRequest request, Metadata headers, CancellationToken cancellationToken)
            {
                return Task.FromResult(new SubmitSignificantEventResponse
                {
                    Meta = new ResponseMeta
                    {
                        RequestId = "req-event",
                        ResultCode = (ResultCode)ProtoResultCode.Ok,
                        DenialReason = string.Empty,
                        ServerTime = "2026-02-15T00:00:00Z",
                    },
                });
            }
        }

        private sealed class FakeReportingRpcClient : IReportingRpcClient
        {
            public Task<GenerateReportResponse> GenerateReportAsync(GenerateReportRequest request, Metadata headers, CancellationToken cancellationToken)
            {
                return Task.FromResult(new GenerateReportResponse
                {
                    Meta = new ResponseMeta
                    {
                        RequestId = "req-report",
                        ResultCode = (ResultCode)ProtoResultCode.Ok,
                        DenialReason = string.Empty,
                        ServerTime = "2026-02-15T00:00:00Z",
                    },
                });
            }
        }
    }
}
