using System;
using System.Threading;
using System.Threading.Tasks;
using WizardBeardStudio.Rgs.Models;

namespace WizardBeardStudio.Rgs.Services
{
    public sealed class WageringClient
    {
        public Task<WagerResult> PlaceWagerAsync(string sessionId, long amountMinor, string currency, string idempotencyKey, CancellationToken cancellationToken)
        {
            throw new NotImplementedException("Wagering client implementation should bridge generated protobuf clients.");
        }

        public Task<OutcomeResult> SettleWagerAsync(string wagerId, long payoutMinor, string currency, CancellationToken cancellationToken)
        {
            throw new NotImplementedException("Wagering client implementation should bridge generated protobuf clients.");
        }
    }
}
