using System;
using System.Collections.Generic;
using System.Net.Http;
using System.Text;
using System.Threading;
using System.Threading.Tasks;

namespace WizardBeardStudio.Rgs.Core
{
    public sealed class RestGatewayRgsTransport : IRgsTransport, IDisposable
    {
        private readonly HttpClient _httpClient;

        public RestGatewayRgsTransport(string baseUrl, int timeoutSeconds)
        {
            _httpClient = new HttpClient
            {
                BaseAddress = new Uri(baseUrl),
                Timeout = TimeSpan.FromSeconds(timeoutSeconds)
            };
        }

        public async Task<string> PostJsonAsync(string path, string jsonBody, IDictionary<string, string>? headers, CancellationToken cancellationToken)
        {
            using var req = new HttpRequestMessage(HttpMethod.Post, path)
            {
                Content = new StringContent(jsonBody, Encoding.UTF8, "application/json")
            };
            ApplyHeaders(req, headers);
            using var resp = await _httpClient.SendAsync(req, cancellationToken);
            return await resp.Content.ReadAsStringAsync(cancellationToken);
        }

        public async Task<string> GetJsonAsync(string path, IDictionary<string, string>? headers, CancellationToken cancellationToken)
        {
            using var req = new HttpRequestMessage(HttpMethod.Get, path);
            ApplyHeaders(req, headers);
            using var resp = await _httpClient.SendAsync(req, cancellationToken);
            return await resp.Content.ReadAsStringAsync(cancellationToken);
        }

        public void Dispose() => _httpClient.Dispose();

        private static void ApplyHeaders(HttpRequestMessage req, IDictionary<string, string>? headers)
        {
            if (headers == null)
            {
                return;
            }
            foreach (var kvp in headers)
            {
                req.Headers.TryAddWithoutValidation(kvp.Key, kvp.Value);
            }
        }
    }
}
