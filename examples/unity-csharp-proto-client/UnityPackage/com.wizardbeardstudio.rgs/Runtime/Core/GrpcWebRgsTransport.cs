using System;
using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;

namespace WizardBeardStudio.Rgs.Core
{
    public sealed class GrpcWebRgsTransport : IRgsTransport, IDisposable
    {
        private readonly IRgsTransport _inner;
        private readonly IDisposable? _disposableInner;

        public GrpcWebRgsTransport(string baseUrl, int timeoutSeconds)
        {
            var rest = new RestGatewayRgsTransport(baseUrl, timeoutSeconds);
            _inner = rest;
            _disposableInner = rest;
        }

        internal GrpcWebRgsTransport(IRgsTransport inner)
        {
            _inner = inner;
            _disposableInner = inner as IDisposable;
        }

        public Task<string> PostJsonAsync(string path, string jsonBody, IDictionary<string, string>? headers, CancellationToken cancellationToken)
        {
            return _inner.PostJsonAsync(path, jsonBody, BuildGrpcWebHeaders(headers), cancellationToken);
        }

        public Task<string> GetJsonAsync(string path, IDictionary<string, string>? headers, CancellationToken cancellationToken)
        {
            return _inner.GetJsonAsync(path, BuildGrpcWebHeaders(headers), cancellationToken);
        }

        public void Dispose()
        {
            _disposableInner?.Dispose();
        }

        private static IDictionary<string, string> BuildGrpcWebHeaders(IDictionary<string, string>? headers)
        {
            var merged = new Dictionary<string, string>(StringComparer.OrdinalIgnoreCase)
            {
                ["x-grpc-web"] = "1",
                ["x-user-agent"] = "grpc-web-csharp/1.0",
            };

            if (headers == null)
            {
                return merged;
            }

            foreach (var kvp in headers)
            {
                merged[kvp.Key] = kvp.Value;
            }

            return merged;
        }
    }
}
