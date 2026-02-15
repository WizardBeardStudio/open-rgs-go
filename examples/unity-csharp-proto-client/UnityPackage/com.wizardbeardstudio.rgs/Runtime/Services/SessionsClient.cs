using System;
using System.Threading;
using System.Threading.Tasks;
using WizardBeardStudio.Rgs.Models;

namespace WizardBeardStudio.Rgs.Services
{
    public sealed class SessionsClient
    {
        public Task<SessionResult> StartSessionAsync(string deviceId, CancellationToken cancellationToken)
        {
            throw new NotImplementedException("Sessions client implementation should bridge generated protobuf clients.");
        }

        public Task<OperationResult> EndSessionAsync(string sessionId, CancellationToken cancellationToken)
        {
            throw new NotImplementedException("Sessions client implementation should bridge generated protobuf clients.");
        }
    }
}
