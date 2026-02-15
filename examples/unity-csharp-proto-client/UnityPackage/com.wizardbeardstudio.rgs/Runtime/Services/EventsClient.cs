using System;
using System.Threading;
using System.Threading.Tasks;
using WizardBeardStudio.Rgs.Models;

namespace WizardBeardStudio.Rgs.Services
{
    public sealed class EventsClient
    {
        public Task<OperationResult> SubmitSignificantEventAsync(string eventCode, string description, CancellationToken cancellationToken)
        {
            throw new NotImplementedException("Events client implementation should bridge generated protobuf clients.");
        }
    }
}
