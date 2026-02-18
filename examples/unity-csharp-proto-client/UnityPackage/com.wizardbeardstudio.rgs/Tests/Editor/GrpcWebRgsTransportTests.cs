using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;
using NUnit.Framework;
using WizardBeardStudio.Rgs.Core;

namespace WizardBeardStudio.Rgs.Tests.Editor
{
    public sealed class GrpcWebRgsTransportTests
    {
        [Test]
        public async Task PostJsonAsync_AddsGrpcWebDefaultHeaders()
        {
            var fake = new CapturingTransport();
            var transport = new GrpcWebRgsTransport(fake);

            await transport.PostJsonAsync("/v1/identity/login", "{}", null, CancellationToken.None);

            Assert.That(fake.LastPostHeaders, Is.Not.Null);
            Assert.That(fake.LastPostHeaders!.TryGetValue("x-grpc-web", out var grpcWeb), Is.True);
            Assert.That(grpcWeb, Is.EqualTo("1"));
            Assert.That(fake.LastPostHeaders!.TryGetValue("x-user-agent", out var userAgent), Is.True);
            Assert.That(userAgent, Is.EqualTo("grpc-web-csharp/1.0"));
        }

        [Test]
        public async Task GetJsonAsync_MergesCallerHeaders()
        {
            var fake = new CapturingTransport();
            var transport = new GrpcWebRgsTransport(fake);
            var callerHeaders = new Dictionary<string, string>
            {
                ["authorization"] = "Bearer token-1",
                ["x-request-id"] = "req-1",
            };

            await transport.GetJsonAsync("/v1/ledger/accounts/a/balance", callerHeaders, CancellationToken.None);

            Assert.That(fake.LastGetHeaders, Is.Not.Null);
            Assert.That(fake.LastGetHeaders!["authorization"], Is.EqualTo("Bearer token-1"));
            Assert.That(fake.LastGetHeaders!["x-request-id"], Is.EqualTo("req-1"));
            Assert.That(fake.LastGetHeaders!["x-grpc-web"], Is.EqualTo("1"));
            Assert.That(fake.LastGetHeaders!["x-user-agent"], Is.EqualTo("grpc-web-csharp/1.0"));
        }

        private sealed class CapturingTransport : IRgsTransport
        {
            public IDictionary<string, string>? LastPostHeaders { get; private set; }
            public IDictionary<string, string>? LastGetHeaders { get; private set; }

            public Task<string> PostJsonAsync(string path, string jsonBody, IDictionary<string, string>? headers, CancellationToken cancellationToken)
            {
                LastPostHeaders = Copy(headers);
                return Task.FromResult("{}");
            }

            public Task<string> GetJsonAsync(string path, IDictionary<string, string>? headers, CancellationToken cancellationToken)
            {
                LastGetHeaders = Copy(headers);
                return Task.FromResult("{}");
            }

            private static IDictionary<string, string> Copy(IDictionary<string, string>? headers)
            {
                return headers == null ? new Dictionary<string, string>() : new Dictionary<string, string>(headers);
            }
        }
    }
}
