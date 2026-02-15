using System;
using System.Net.Http;
using System.Threading.Tasks;
using Grpc.Core;
using Grpc.Net.Client;
using Grpc.Net.Client.Web;
using Rgs.V1;
using UnityEngine;

public sealed class RgsClientBehaviour : MonoBehaviour
{
    [Header("RGS Endpoint")]
    [SerializeField] private string baseUrl = "https://localhost:8080";

    [Header("Player Credentials")]
    [SerializeField] private string playerId = "player-1";
    [SerializeField] private string playerPin = "1234";
    [SerializeField] private string accountId = "acct-player-1";

    [Header("Client Metadata")]
    [SerializeField] private string deviceId = "unity-slot-client-01";

    private GrpcChannel? _channel;
    private IdentityService.IdentityServiceClient? _identityClient;
    private LedgerService.LedgerServiceClient? _ledgerClient;
    private SessionToken? _token;

    private async void Start()
    {
        try
        {
            InitializeChannel();
            await LoginAsync();
            await FetchBalanceAsync();
        }
        catch (RpcException rpcEx)
        {
            Debug.LogError($"RGS RPC error: {rpcEx.StatusCode} {rpcEx.Status.Detail}");
        }
        catch (Exception ex)
        {
            Debug.LogError($"RGS client failed: {ex.Message}");
        }
    }

    private void OnDestroy()
    {
        _channel?.Dispose();
        _channel = null;
    }

    private void InitializeChannel()
    {
        var grpcWebHandler = new GrpcWebHandler(GrpcWebMode.GrpcWebText, new HttpClientHandler());
        _channel = GrpcChannel.ForAddress(baseUrl, new GrpcChannelOptions { HttpHandler = grpcWebHandler });
        _identityClient = new IdentityService.IdentityServiceClient(_channel);
        _ledgerClient = new LedgerService.LedgerServiceClient(_channel);
    }

    private async Task LoginAsync()
    {
        if (_identityClient == null)
        {
            throw new InvalidOperationException("identity client is not initialized");
        }

        var request = new LoginRequest
        {
            Meta = BuildMeta(string.Empty),
            Player = new PlayerCredentials
            {
                PlayerId = playerId,
                Pin = playerPin,
            }
        };

        var response = await _identityClient.LoginAsync(request);
        EnsureOk(response.Meta, "login");
        _token = response.Token;
        Debug.Log($"RGS login succeeded for {playerId}");
    }

    private async Task FetchBalanceAsync()
    {
        if (_ledgerClient == null)
        {
            throw new InvalidOperationException("ledger client is not initialized");
        }
        if (_token == null || string.IsNullOrWhiteSpace(_token.AccessToken))
        {
            throw new InvalidOperationException("token is not available");
        }

        var headers = new Metadata
        {
            { "authorization", $"Bearer {_token.AccessToken}" }
        };

        var request = new GetBalanceRequest
        {
            Meta = BuildMeta(string.Empty),
            AccountId = accountId
        };

        var response = await _ledgerClient.GetBalanceAsync(request, headers);
        EnsureOk(response.Meta, "get balance");

        var amount = response.AvailableBalance?.AmountMinor ?? 0;
        var currency = response.AvailableBalance?.Currency ?? "UNK";
        Debug.Log($"RGS balance: {amount} {currency} (minor units)");
    }

    private RequestMeta BuildMeta(string idempotencyKey)
    {
        return new RequestMeta
        {
            RequestId = Guid.NewGuid().ToString(),
            IdempotencyKey = idempotencyKey,
            Actor = new Actor
            {
                ActorId = playerId,
                ActorType = ActorType.Player
            },
            Source = new Source
            {
                DeviceId = deviceId,
                UserAgent = "unity-slot-client",
                Ip = string.Empty,
                Geo = string.Empty
            }
        };
    }

    private static void EnsureOk(ResponseMeta? meta, string operation)
    {
        if (meta == null)
        {
            throw new InvalidOperationException($"{operation}: missing response meta");
        }
        if (meta.ResultCode != ResultCode.Ok)
        {
            throw new InvalidOperationException($"{operation}: {meta.ResultCode} {meta.DenialReason}");
        }
    }
}
