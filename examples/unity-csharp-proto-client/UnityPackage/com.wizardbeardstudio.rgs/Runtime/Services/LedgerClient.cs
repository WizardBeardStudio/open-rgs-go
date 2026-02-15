using System;
using System.Threading;
using System.Threading.Tasks;
using WizardBeardStudio.Rgs.Models;

namespace WizardBeardStudio.Rgs.Services
{
    public sealed class LedgerClient
    {
        public Task<BalanceResult> GetBalanceAsync(string accountId, CancellationToken cancellationToken)
        {
            throw new NotImplementedException("Ledger client implementation should bridge generated protobuf clients.");
        }
    }
}
