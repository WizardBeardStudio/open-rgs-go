using System;
using System.Collections.Generic;
#if UNITY_WEBGL && !UNITY_EDITOR
using System.Text;
#else
using System.Net.Http;
using System.Text;
#endif
using System.Threading;
using System.Threading.Tasks;
#if UNITY_WEBGL && !UNITY_EDITOR
using UnityEngine.Networking;
#endif

namespace WizardBeardStudio.Rgs.Core
{
    public sealed class RestGatewayRgsTransport : IRgsTransport, IDisposable
    {
#if UNITY_WEBGL && !UNITY_EDITOR
        private readonly Uri _baseUri;
#else
        private readonly HttpClient _httpClient;
#endif

        public RestGatewayRgsTransport(string baseUrl, int timeoutSeconds)
        {
#if UNITY_WEBGL && !UNITY_EDITOR
            _baseUri = new Uri(baseUrl, UriKind.Absolute);
#else
            _httpClient = new HttpClient
            {
                BaseAddress = new Uri(baseUrl),
                Timeout = TimeSpan.FromSeconds(timeoutSeconds)
            };
#endif
        }

        public async Task<string> PostJsonAsync(string path, string jsonBody, IDictionary<string, string>? headers, CancellationToken cancellationToken)
        {
#if UNITY_WEBGL && !UNITY_EDITOR
            var bytes = Encoding.UTF8.GetBytes(jsonBody);
            using var req = new UnityWebRequest(BuildUrl(path), UnityWebRequest.kHttpVerbPOST)
            {
                uploadHandler = new UploadHandlerRaw(bytes),
                downloadHandler = new DownloadHandlerBuffer()
            };
            req.SetRequestHeader("Content-Type", "application/json");
            ApplyHeaders(req, headers);
            await SendAsync(req, cancellationToken);
            return req.downloadHandler.text;
#else
            using var req = new HttpRequestMessage(HttpMethod.Post, path)
            {
                Content = new StringContent(jsonBody, Encoding.UTF8, "application/json")
            };
            ApplyHeaders(req, headers);
            using var resp = await _httpClient.SendAsync(req, cancellationToken);
            return await resp.Content.ReadAsStringAsync(cancellationToken);
#endif
        }

        public async Task<string> GetJsonAsync(string path, IDictionary<string, string>? headers, CancellationToken cancellationToken)
        {
#if UNITY_WEBGL && !UNITY_EDITOR
            using var req = UnityWebRequest.Get(BuildUrl(path));
            ApplyHeaders(req, headers);
            await SendAsync(req, cancellationToken);
            return req.downloadHandler.text;
#else
            using var req = new HttpRequestMessage(HttpMethod.Get, path);
            ApplyHeaders(req, headers);
            using var resp = await _httpClient.SendAsync(req, cancellationToken);
            return await resp.Content.ReadAsStringAsync(cancellationToken);
#endif
        }

        public void Dispose()
        {
#if !UNITY_WEBGL || UNITY_EDITOR
            _httpClient.Dispose();
#endif
        }

#if UNITY_WEBGL && !UNITY_EDITOR
        private string BuildUrl(string path) => new Uri(_baseUri, path).ToString();

        private static void ApplyHeaders(UnityWebRequest req, IDictionary<string, string>? headers)
        {
            if (headers == null)
            {
                return;
            }
            foreach (var kvp in headers)
            {
                req.SetRequestHeader(kvp.Key, kvp.Value);
            }
        }

        private static async Task SendAsync(UnityWebRequest req, CancellationToken cancellationToken)
        {
            using var registration = cancellationToken.Register(req.Abort);
            var op = req.SendWebRequest();
            while (!op.isDone)
            {
                cancellationToken.ThrowIfCancellationRequested();
                await Task.Yield();
            }
            if (req.result == UnityWebRequest.Result.ConnectionError ||
                req.result == UnityWebRequest.Result.ProtocolError ||
                req.result == UnityWebRequest.Result.DataProcessingError)
            {
                throw new InvalidOperationException($"REST request failed: {req.responseCode} {req.error}");
            }
        }
#else
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
#endif
    }
}
