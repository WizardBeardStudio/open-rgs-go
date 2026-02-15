using System;
using System.Collections.Generic;
using System.Threading;
using System.Threading.Tasks;

namespace WizardBeardStudio.Rgs.Core
{
    public sealed class GrpcWebRgsTransport : IRgsTransport
    {
        public Task<string> PostJsonAsync(string path, string jsonBody, IDictionary<string, string>? headers, CancellationToken cancellationToken)
        {
            throw new NotImplementedException("gRPC-Web transport wiring is package TODO. Use service-specific generated clients in initial integration.");
        }

        public Task<string> GetJsonAsync(string path, IDictionary<string, string>? headers, CancellationToken cancellationToken)
        {
            throw new NotImplementedException("gRPC-Web transport wiring is package TODO. Use service-specific generated clients in initial integration.");
        }
    }
}
