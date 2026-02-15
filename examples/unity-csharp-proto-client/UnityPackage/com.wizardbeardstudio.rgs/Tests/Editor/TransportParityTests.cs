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
    public sealed class TransportParityTests
    {
        [Test]
        public async Task IdentityLogin_GrpcAndRest_MapSameCoreResult()
        {
            var grpcClient = new IdentityClient(new FakeIdentityRpcClient(), "dev-1", "ua", string.Empty);
            var restClient = new IdentityClient(new FakeTransport(), "dev-1", "ua", string.Empty);

            var grpc = await grpcClient.LoginPlayerAsync("player-1", "1234", CancellationToken.None);
            var rest = await restClient.LoginPlayerAsync("player-1", "1234", CancellationToken.None);

            Assert.That(rest.Success, Is.EqualTo(grpc.Success));
            Assert.That(rest.ResultCode, Is.Not.Empty);
            Assert.That(grpc.ResultCode, Is.Not.Empty);
            Assert.That(rest.ActorId, Is.EqualTo(grpc.ActorId));
            Assert.That(rest.AccessToken, Is.EqualTo(grpc.AccessToken));
        }

        [Test]
        public async Task LedgerGetBalance_GrpcAndRest_MapSameCoreResult()
        {
            var grpcClient = new LedgerClient(new FakeLedgerRpcClient(), () => "token", "player-1", "dev-1", "ua", string.Empty);
            var restClient = new LedgerClient(new FakeTransport(), () => "token", "player-1", "dev-1", "ua", string.Empty);

            var grpc = await grpcClient.GetBalanceAsync("acct-player-1", CancellationToken.None);
            var rest = await restClient.GetBalanceAsync("acct-player-1", CancellationToken.None);

            Assert.That(rest.Success, Is.EqualTo(grpc.Success));
            Assert.That(rest.AvailableMinor, Is.EqualTo(grpc.AvailableMinor));
            Assert.That(rest.Currency, Is.EqualTo(grpc.Currency));
        }

        [Test]
        public async Task SessionsStartEnd_GrpcAndRest_MapSameCoreResult()
        {
            var grpcClient = new SessionsClient(new FakeSessionsRpcClient(), () => "token", "player-1", "dev-1", "ua", string.Empty);
            var restClient = new SessionsClient(new FakeTransport(), () => "token", "player-1", "dev-1", "ua", string.Empty);

            var grpcStart = await grpcClient.StartSessionAsync("dev-1", CancellationToken.None);
            var restStart = await restClient.StartSessionAsync("dev-1", CancellationToken.None);
            Assert.That(restStart.Success, Is.EqualTo(grpcStart.Success));
            Assert.That(restStart.SessionId, Is.EqualTo(grpcStart.SessionId));

            var grpcEnd = await grpcClient.EndSessionAsync("sess-1", CancellationToken.None);
            var restEnd = await restClient.EndSessionAsync("sess-1", CancellationToken.None);
            Assert.That(restEnd.Success, Is.EqualTo(grpcEnd.Success));
        }

        [Test]
        public async Task WageringPlaceSettle_GrpcAndRest_MapSameCoreResult()
        {
            var grpcClient = new WageringClient(new FakeWageringRpcClient(), () => "token", "player-1", "dev-1", "ua", string.Empty);
            var restClient = new WageringClient(new FakeTransport(), () => "token", "player-1", "dev-1", "ua", string.Empty);

            var grpcPlace = await grpcClient.PlaceWagerAsync("slot-1", 100, "USD", "idem-1", CancellationToken.None);
            var restPlace = await restClient.PlaceWagerAsync("slot-1", 100, "USD", "idem-1", CancellationToken.None);
            Assert.That(restPlace.Success, Is.EqualTo(grpcPlace.Success));
            Assert.That(restPlace.WagerId, Is.EqualTo(grpcPlace.WagerId));

            var grpcSettle = await grpcClient.SettleWagerAsync("w-1", 150, "USD", CancellationToken.None);
            var restSettle = await restClient.SettleWagerAsync("w-1", 150, "USD", CancellationToken.None);
            Assert.That(restSettle.Success, Is.EqualTo(grpcSettle.Success));
            Assert.That(restSettle.WagerId, Is.EqualTo(grpcSettle.WagerId));
        }

        private sealed class FakeTransport : IRgsTransport
        {
            public Task<string> PostJsonAsync(string path, string jsonBody, IDictionary<string, string>? headers, CancellationToken cancellationToken)
            {
                if (path == "/v1/identity/login")
                {
                    return Task.FromResult("{" +
                                           "\"meta\":{\"requestId\":\"req-login\",\"resultCode\":\"RESULT_CODE_OK\",\"denialReason\":\"\",\"serverTime\":\"2026-02-15T00:00:00Z\"}," +
                                           "\"token\":{\"accessToken\":\"access\",\"refreshToken\":\"refresh\",\"actor\":{\"actorId\":\"player-1\"}}}");
                }
                if (path == "/v1/sessions:start")
                {
                    return Task.FromResult("{" +
                                           "\"meta\":{\"requestId\":\"req-start\",\"resultCode\":\"RESULT_CODE_OK\",\"denialReason\":\"\",\"serverTime\":\"2026-02-15T00:00:00Z\"}," +
                                           "\"session\":{\"sessionId\":\"sess-1\",\"state\":\"SESSION_STATE_ACTIVE\"}}" );
                }
                if (path == "/v1/sessions:end")
                {
                    return Task.FromResult("{" +
                                           "\"meta\":{\"requestId\":\"req-end\",\"resultCode\":\"RESULT_CODE_OK\",\"denialReason\":\"\",\"serverTime\":\"2026-02-15T00:00:00Z\"}}" );
                }
                if (path == "/v1/wagering/wagers")
                {
                    return Task.FromResult("{" +
                                           "\"meta\":{\"requestId\":\"req-place\",\"resultCode\":\"RESULT_CODE_OK\",\"denialReason\":\"\",\"serverTime\":\"2026-02-15T00:00:00Z\"}," +
                                           "\"wager\":{\"wagerId\":\"w-1\",\"status\":\"WAGER_STATUS_PENDING\"}}" );
                }
                if (path.Contains(":settle"))
                {
                    return Task.FromResult("{" +
                                           "\"meta\":{\"requestId\":\"req-settle\",\"resultCode\":\"RESULT_CODE_OK\",\"denialReason\":\"\",\"serverTime\":\"2026-02-15T00:00:00Z\"}," +
                                           "\"wager\":{\"wagerId\":\"w-1\",\"status\":\"WAGER_STATUS_SETTLED\"}}" );
                }
                return Task.FromResult("{\"meta\":{\"requestId\":\"req-x\",\"resultCode\":\"RESULT_CODE_OK\",\"denialReason\":\"\",\"serverTime\":\"2026-02-15T00:00:00Z\"}}" );
            }

            public Task<string> GetJsonAsync(string path, IDictionary<string, string>? headers, CancellationToken cancellationToken)
            {
                if (path == "/v1/ledger/accounts/acct-player-1/balance")
                {
                    return Task.FromResult("{" +
                                           "\"meta\":{\"requestId\":\"req-bal\",\"resultCode\":\"RESULT_CODE_OK\",\"denialReason\":\"\",\"serverTime\":\"2026-02-15T00:00:00Z\"}," +
                                           "\"availableBalance\":{\"amountMinor\":2500,\"currency\":\"USD\"}," +
                                           "\"pendingBalance\":{\"amountMinor\":0,\"currency\":\"USD\"}}" );
                }
                return Task.FromResult("{\"meta\":{\"requestId\":\"req-g\",\"resultCode\":\"RESULT_CODE_OK\",\"denialReason\":\"\",\"serverTime\":\"2026-02-15T00:00:00Z\"}}" );
            }
        }

        private sealed class FakeIdentityRpcClient : IIdentityRpcClient
        {
            public Task<LoginResponse> LoginAsync(LoginRequest request, CancellationToken cancellationToken)
            {
                return Task.FromResult(new LoginResponse
                {
                    Meta = new ResponseMeta
                    {
                        RequestId = "req-login",
                        ResultCode = (ResultCode)ProtoResultCode.Ok,
                        DenialReason = string.Empty,
                        ServerTime = "2026-02-15T00:00:00Z",
                    },
                    Token = new SessionToken
                    {
                        AccessToken = "access",
                        RefreshToken = "refresh",
                        Actor = new Actor { ActorId = "player-1", ActorType = (ActorType)1 }
                    }
                });
            }

            public Task<RefreshTokenResponse> RefreshTokenAsync(RefreshTokenRequest request, CancellationToken cancellationToken)
            {
                return Task.FromResult(new RefreshTokenResponse
                {
                    Meta = new ResponseMeta
                    {
                        RequestId = "req-refresh",
                        ResultCode = (ResultCode)ProtoResultCode.Ok,
                        DenialReason = string.Empty,
                        ServerTime = "2026-02-15T00:00:00Z",
                    },
                    Token = new SessionToken
                    {
                        AccessToken = "access-2",
                        RefreshToken = "refresh-2",
                        Actor = new Actor { ActorId = "player-1", ActorType = (ActorType)1 }
                    }
                });
            }

            public Task<LogoutResponse> LogoutAsync(LogoutRequest request, CancellationToken cancellationToken)
            {
                return Task.FromResult(new LogoutResponse
                {
                    Meta = new ResponseMeta
                    {
                        RequestId = "req-logout",
                        ResultCode = (ResultCode)ProtoResultCode.Ok,
                        DenialReason = string.Empty,
                        ServerTime = "2026-02-15T00:00:00Z",
                    }
                });
            }
        }

        private sealed class FakeLedgerRpcClient : ILedgerRpcClient
        {
            public Task<GetBalanceResponse> GetBalanceAsync(GetBalanceRequest request, Metadata headers, CancellationToken cancellationToken)
            {
                return Task.FromResult(new GetBalanceResponse
                {
                    Meta = new ResponseMeta
                    {
                        RequestId = "req-bal",
                        ResultCode = (ResultCode)ProtoResultCode.Ok,
                        DenialReason = string.Empty,
                        ServerTime = "2026-02-15T00:00:00Z",
                    },
                    AvailableBalance = new Money { AmountMinor = 2500, Currency = "USD" },
                    PendingBalance = new Money { AmountMinor = 0, Currency = "USD" }
                });
            }
        }

        private sealed class FakeSessionsRpcClient : ISessionsRpcClient
        {
            public Task<StartSessionResponse> StartSessionAsync(StartSessionRequest request, Metadata headers, CancellationToken cancellationToken)
            {
                return Task.FromResult(new StartSessionResponse
                {
                    Meta = new ResponseMeta
                    {
                        RequestId = "req-start",
                        ResultCode = (ResultCode)ProtoResultCode.Ok,
                        DenialReason = string.Empty,
                        ServerTime = "2026-02-15T00:00:00Z",
                    },
                    Session = new PlayerSession { SessionId = "sess-1", State = (SessionState)1 }
                });
            }

            public Task<EndSessionResponse> EndSessionAsync(EndSessionRequest request, Metadata headers, CancellationToken cancellationToken)
            {
                return Task.FromResult(new EndSessionResponse
                {
                    Meta = new ResponseMeta
                    {
                        RequestId = "req-end",
                        ResultCode = (ResultCode)ProtoResultCode.Ok,
                        DenialReason = string.Empty,
                        ServerTime = "2026-02-15T00:00:00Z",
                    }
                });
            }
        }

        private sealed class FakeWageringRpcClient : IWageringRpcClient
        {
            public Task<PlaceWagerResponse> PlaceWagerAsync(PlaceWagerRequest request, Metadata headers, CancellationToken cancellationToken)
            {
                return Task.FromResult(new PlaceWagerResponse
                {
                    Meta = new ResponseMeta
                    {
                        RequestId = "req-place",
                        ResultCode = (ResultCode)ProtoResultCode.Ok,
                        DenialReason = string.Empty,
                        ServerTime = "2026-02-15T00:00:00Z",
                    },
                    Wager = new Wager { WagerId = "w-1", Status = (WagerStatus)1 }
                });
            }

            public Task<SettleWagerResponse> SettleWagerAsync(SettleWagerRequest request, Metadata headers, CancellationToken cancellationToken)
            {
                return Task.FromResult(new SettleWagerResponse
                {
                    Meta = new ResponseMeta
                    {
                        RequestId = "req-settle",
                        ResultCode = (ResultCode)ProtoResultCode.Ok,
                        DenialReason = string.Empty,
                        ServerTime = "2026-02-15T00:00:00Z",
                    },
                    Wager = new Wager { WagerId = "w-1", Status = (WagerStatus)2 }
                });
            }
        }
    }
}
