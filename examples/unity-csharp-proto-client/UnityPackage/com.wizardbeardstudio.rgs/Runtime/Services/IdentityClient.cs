using System;
using System.Threading;
using System.Threading.Tasks;
using WizardBeardStudio.Rgs.Models;

namespace WizardBeardStudio.Rgs.Services
{
    public sealed class IdentityClient
    {
        public Task<LoginResult> LoginPlayerAsync(string playerId, string pin, CancellationToken cancellationToken)
        {
            throw new NotImplementedException("Identity client implementation should bridge generated protobuf clients.");
        }
    }
}
